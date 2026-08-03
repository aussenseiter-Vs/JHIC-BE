---
name: Nexxa Domain Documentation
relation: RULES.md → modules/nexxa/
description: Documentation for the nexxa domain — AI-powered chatbot, Nexxa-Match recommendation, and stateless transforms
type: editable
---

# Nexxa Domain

## Overview

The `nexxa` domain exposes n8n's AI webhooks through the JHIC-BE backend so the frontend never talks to n8n directly. It is organized into sub-domains: `chat` (chatbot) and `match` (Nexxa-Match recommendation + stateless transforms). Each sub-domain is a separate Go package under `internal/domain/nexxa/`. The domain validates incoming payloads, forwards them server-side to the n8n webhook, and relays the response as-is. Chat/Nexxa endpoints are public (no auth) and rate-limited per client IP; the two stateless transforms are public and not rate-limited because they sit in n8n's synchronous webhook critical path. This domain has no database — the `N8NClient` interface (defined in the parent `nexxa` package) plays the role a repository would, and its implementation lives in `internal/infrastructure/n8n/`.

## Structure

```
internal/domain/nexxa/
├── client.go          — N8NClient interface (shared by both sub-domains)
├── entity.go          — ChatResponse (shared type needed by the interface)
├── errors.go          — ErrN8NUnavailable, ErrN8NTimeout (shared upstream errors)
├── chat/              — chat sub-domain
│   ├── entity.go      — ChatRequest, ChatMessageMaxLen
│   ├── errors.go      — ErrChatMessageRequired, ErrChatMessageTooLong
│   ├── service.go     — Chat business logic
│   ├── handler.go     — Chat handler + Register
│   └── service_test.go
├── match/             — nexxa-match sub-domain
│   ├── entity.go      — NexxaRequest, NexxaResponse, APIError, etc.
│   ├── errors.go      — ErrAnswersRequired, ErrAnswerTooLong, ErrNexxaOutputInvalid
│   ├── nexxa.go       — pure stateless functions (sanitize, validate, normalize)
│   ├── nexxa_log.go   — SHA-256-only structured logging
│   ├── service.go     — NexxaMatch + validate/normalize service methods
│   ├── handler.go     — NexxaMatch, ValidateNexxaInput, NormalizeNexxaOutput handlers
│   ├── service_test.go
│   ├── nexxa_handler_test.go
│   └── nexxa_test.go
└── mocks/
    └── N8NClient.go   — mockery-generated mock of N8NClient
```

## Entity

### Parent package (nexxa)

```go
type ChatResponse struct {
    Output string `json:"output"`
}
```

### chat sub-domain

```go
type ChatRequest struct {
    ChatInput string `json:"chatInput"`
    SessionID string `json:"sessionId"`
}
```

### match sub-domain

```go
type NexxaRequest struct {
    Jawaban1 string `json:"jawaban_1"`
    // ... Jawaban2 .. Jawaban8
}

type NexxaResponse struct {
    NamaJurusan         string `json:"nama_jurusan"`
    Alasan              string `json:"alasan"`
    PersentasePPLG      int    `json:"persentase_pplg"`
    PersentaseAkuntansi int    `json:"persentase_akuntansi"`
    PersentaseHotel     int    `json:"persentase_hotel"`
}

type APIError struct {
    Field   string `json:"field,omitempty"`
    Message string `json:"message"`
}

type ValidateInputData map[string]string

type NormalizeOutputRequest struct {
    Raw string `json:"raw"`
}

type NormalizeOutputData struct {
    NamaJurusan         string `json:"nama_jurusan"`
    Alasan              string `json:"alasan"`
    PersentasePPLG      int    `json:"persentase_pplg"`
    PersentaseAkuntansi int    `json:"persentase_akuntansi"`
    PersentaseHotel     int    `json:"persentase_hotel"`
}
```

## Endpoints

Chat/Nexxa endpoints are public, rate-limited (default 10 req/min/IP), and capped at 32KB bodies. The two stateless transforms are public, not rate-limited, and capped at 32KB bodies.

| Method | Path | Sub-domain | Description |
|---|---|---|---|
| POST | /api/v1/nexxa/chat | chat | Validate + forward a chatbot message to n8n, relay `{output}` |
| POST | /api/v1/nexxa/match | match | Validate 8 answers + forward to n8n, relay `{nama_jurusan, alasan}` |
| POST | /api/v1/nexxa/match/validate-input | match | Sanitize + validate the 8 raw student answers before the LLM call |
| POST | /api/v1/nexxa/match/normalize-output | match | Parse + repair the model's JSON output after the LLM call |

## Data flow

### Chat

```
POST /api/v1/nexxa/chat {chatInput, sessionId}
  → middleware.RateLimit → chat.Handler.Chat: MaxBytesReader + JSON decode
    → chat.Service.Chat: trim + require non-empty + ≤300 chars; generate sessionId if empty
      → n8n.Client.Chat: POST {chatInput, sessionId} to N8N_CHAT_PATH
        (Basic Auth header from N8N_CHAT_USERNAME/PASSWORD)
        → relay {output} as-is, or 502/504 on upstream failure
```

### Nexxa-Match

```
POST /api/v1/nexxa/match {jawaban_1..8}
  → middleware.RateLimit → match.Handler.NexxaMatch: MaxBytesReader + JSON decode
    → match.Service.NexxaMatch: require exactly 8 answers, each non-empty and ≤500 chars
      → n8n.Client.NexxaMatch: POST jawaban_1..8 to N8N_NEXXA_PATH
        (X-JHIC-Secret header from N8N_NEXXA_SECRET)
        → relay {nama_jurusan, alasan} as-is, or 502/504 on upstream failure
```

### Validate Input

```
POST /api/v1/nexxa/match/validate-input {jawaban_1..8}
  → match.Handler.ValidateNexxaInput: MaxBytesReader + decode to map[string]json.RawMessage
    → match.Service.ValidateNexxaInput:
        - each jawaban_N must be present, a plain string, non-empty after trim, ≤500 chars
        - sanitize: strip <script>/<style> blocks + HTML tags, collapse whitespace to single spaces, trim
        - flag prompt-injection patterns (ignore previous instructions, system:, you are now, ...) for logs
    → 200 {success:true, data:{jawaban_1..8: sanitized}} or 400 {success:false, errors:[{field,message}]}
```

### Normalize Output

```
POST /api/v1/nexxa/match/normalize-output {raw}
  → match.Handler.NormalizeNexxaOutput: MaxBytesReader + JSON decode
    → match.Service.NormalizeNexxaOutput:
        - strip ```json / ``` fences and stray prose, try direct JSON parse, else extract first {…} block
        - validate nama_jurusan ∈ {PPLG, Akuntansi, Perhotelan} (case/punctuation tolerant, e.g. pplg, P.P.L.G)
        - require non-empty alasan; require each percentage ∈ [0,100]
        - rescale percentages so they sum to exactly 100, adjusting the largest value by the rounding remainder
    → 200 {success:true, data:{nama_jurusan, alasan, persentase_pplg, persentase_akuntansi, persentase_hotel}}
      or 422 {success:false, errors:[{message}]} — never throws on unparseable input
```

The service propagates the request context into the upstream call, so a browser disconnect cancels the n8n execution mid-flight. The upstream client times out via `N8N_TIMEOUT` (default 115s); the server `WriteTimeout` is 120s so slow LLM responses are not cut off.

## Rules

- Input validation is business logic in the service, not the handler.
- `chatInput` is trimmed before length checks and forwarding; `sessionId` is auto-generated (32 hex chars) when absent.
- Nexxa requires exactly 8 answers; answers are trimmed and forwarded normalized.
- The two stateless transforms are pure — no DB, no upstream calls — and complete in well under 500ms.
- `validate-input` never truncates silently: oversized fields are rejected with `400` so the frontend can message the user.
- `normalize-output` returns `422` for unparseable model output; it never leaks raw model output into logs — student answers and raw model text are logged only as SHA-256 prefixes + lengths.
- `normalize-output` percentages always sum to exactly 100 after rescaling; a zero/missing total is a `422` (never guessed).
- Upstream HTTP errors map to `502 Bad Gateway`; timeouts to `504 Gateway Timeout`. Neither leaks upstream response bodies.
- The n8n chat webhook is authenticated with Basic Auth; the Nexxa webhook with the `X-JHIC-Secret` header — both values come from env and are sent only when configured.
- Rate limiting is a token bucket keyed by client IP with no background goroutine (opportunistic cleanup only).

## cURL examples

```bash
# Chatbot
curl -X POST http://localhost:8080/api/v1/nexxa/chat \
  -H 'Content-Type: application/json' \
  -d '{"chatInput":"Bagaimana cara mendaftar PPDB?","sessionId":"123e4567-e89b-12d3-a456-426614174000"}'

# Nexxa-Match
curl -X POST http://localhost:8080/api/v1/nexxa/match \
  -H 'Content-Type: application/json' \
  -d '{"jawaban_1":"a","jawaban_2":"b","jawaban_3":"c","jawaban_4":"d","jawaban_5":"e","jawaban_6":"f","jawaban_7":"g","jawaban_8":"h"}'

# Validate input (sanitize before the LLM call)
curl -X POST http://localhost:8080/api/v1/nexxa/match/validate-input \
  -H 'Content-Type: application/json' \
  -d '{"jawaban_1":"Saya <b>suka</b> komputer","jawaban_2":"b","jawaban_3":"c","jawaban_4":"d","jawaban_5":"e","jawaban_6":"f","jawaban_7":"g","jawaban_8":"h"}'

# Normalize output (repair after the LLM call)
curl -X POST http://localhost:8080/api/v1/nexxa/match/normalize-output \
  -H 'Content-Type: application/json' \
  -d '{"raw":"```json\n{\"nama_jurusan\":\"PPLG\",\"alasan\":\"cocok\",\"persentase_pplg\":70,\"persentase_akuntansi\":20,\"persentase_hotel\":20}\n```"}'
```