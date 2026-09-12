---
name: Nexxa Domain Documentation
relation: RULES.md → modules/nexxa/
description: Documentation for the nexxa domain — AI chatbot, Nexxa-Match recommendation, CV review, and stateless transforms
type: editable
---

# Nexxa Domain

## Overview

The `nexxa` domain is JHIC-BE's AI feature set: `chat` (a school Q&A chatbot), `match` (Nexxa-Match major recommendation + stateless transforms), and `cvreview` (CV audit + stateless transforms). Each sub-domain is a separate Go package under `internal/domain/nexxa/`. Instead of proxying n8n webhooks, the domain now calls an LLM directly through the `AIClient` interface (defined in the parent `nexxa` package), implemented by `internal/infrastructure/llm/` as a generic OpenAI-compatible client.

The chat sub-domain additionally implements retrieval-augmented generation (RAG) over a local knowledge base: it embeds the latest question with a local Hugging Face TEI model, retrieves the most similar chunks from a pgvector-enabled Postgres table, and grounds the LLM reply on them. Per-session chat history is kept in process memory (bounded, TTL-based) — nothing is written to the database.

Chat/Nexxa endpoints are public (no auth) and rate-limited per client IP; the CV review main endpoint requires auth and is rate-limited; the four stateless transforms are public and not rate-limited because they sit in the request critical path.

## Structure

```
internal/domain/nexxa/
├── client.go          — AIClient interface + Message (shared by all sub-domains)
├── entity.go          — ChatResponse (shared type needed by the interface)
├── errors.go          — ErrUpstreamUnavailable, ErrUpstreamTimeout (shared upstream errors)
├── chat/              — chat sub-domain
│   ├── entity.go      — ChatRequest, ChatMessageMaxLen
│   ├── errors.go      — ErrChatMessageRequired, ErrChatMessageTooLong
│   ├── memory.go      — in-process per-session chat history (bounded + TTL)
│   ├── repository.go  — Chunk, KnowledgeRepository, Embedder interfaces
│   ├── prompt.go      — chat system prompt + BuildMessages (RAG-aware)
│   ├── service.go     — Chat business logic (RAG + memory + LLM)
│   ├── handler.go     — Chat handler + Register
│   ├── pg/            — knowledge-base pgvector repository
│   ├── mocks/         — KnowledgeRepository, Embedder mocks
│   ├── service_test.go
│   └── memory_test.go
├── match/             — nexxa-match sub-domain
│   ├── entity.go      — NexxaRequest, NexxaResponse, APIError, etc.
│   ├── errors.go      — ErrAnswersRequired, ErrAnswerTooLong, ErrNexxaOutputInvalid
│   ├── prompt.go      — recommendation system prompt + MatchMessages
│   ├── content/       — pure stateless functions (sanitize, validate, normalize)
│   ├── service.go     — NexxaMatch + validate/normalize service methods
│   ├── handler.go     — NexxaMatch, ValidateNexxaInput, NormalizeNexxaOutput handlers
│   ├── service_test.go
│   └── handler_test.go
├── cvreview/          — cv-review sub-domain
│   ├── entity.go      — CvReviewRequest, ValidateInputRequest, NormalizeOutputRequest
│   ├── errors.go      — ErrCvTextRequired, ErrCvTextTooLong, ErrInvalidCounts, ErrCvOutputInvalid
│   ├── prompt.go      — CV audit system prompt + CvReviewMessages
│   ├── content/       — pure functions (sanitize, validate, normalize the AI JSON schema)
│   ├── service.go     — CvReview + validate/normalize service methods
│   ├── handler.go     — CvReview, ValidateCvInput, NormalizeCvOutput handlers
│   ├── service_test.go
│   └── handler_test.go
├── pg/                — knowledge base pgvector repository tests (chat/pg)
└── mocks/
    └── AIClient.go    — mock of AIClient (testify style)
```

The upstream client and knowledge-base seeder live outside the domain:

```
internal/infrastructure/llm/        — OpenAI-compatible chat completions + embeddings client
internal/infrastructure/knowledge/ — markdown chunker, embedder, and KB seeder
cmd/kbseed/                         — CLI that (re)indexes the KB from a directory
```

## Entity

### Parent package (nexxa)

```go
type Message struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant"
    Content string `json:"content"`
}

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

type Chunk struct {
    DocumentTitle string
    Content       string
}
```

## Endpoints

Chat/Nexxa endpoints are public, rate-limited (default 10 req/min/IP), and capped at 32KB bodies. The four stateless transforms are public, not rate-limited, and capped at 32KB bodies (1MB for the cv-review ones). The cv-review main endpoint requires auth (Bearer token) and is rate-limited; its body cap is 1MB to fit raw CV text.

| Method | Path | Sub-domain | Description |
|---|---|---|---|
| POST | /api/v1/nexxa/chat | chat | RAG + memory chatbot, returns `{output}` |
| POST | /api/v1/nexxa/match | match | Validate 8 answers + LLM recommendation, returns `{nama_jurusan, alasan}` |
| POST | /api/v1/nexxa/match/validate-input | match | Sanitize + validate the 8 raw student answers before the LLM call |
| POST | /api/v1/nexxa/match/normalize-output | match | Parse + repair the model's JSON output after the LLM call |
| POST | /api/v1/nexxa/cv-review | cvreview | Auth + rate-limited. Validate CV input, LLM audit, normalize the CV audit JSON |
| POST | /api/v1/nexxa/cv-review/validate-input | cvreview | Sanitize + validate raw CV text and word/page counts before the LLM call |
| POST | /api/v1/nexxa/cv-review/normalize-output | cvreview | Parse + repair the CV audit model JSON after the LLM call |

## Data flow

### Chat (RAG + memory)

```
POST /api/v1/nexxa/chat {chatInput, sessionId, topic?}
  → middleware.RateLimit → chat.Handler.Chat: MaxBytesReader + JSON decode
    → chat.Service.Chat: trim + require non-empty + ≤300 chars; generate sessionId if empty
      → chat.Service.retrieveContext:
          - llm.Client.Embed([chatInput])  → 384-dim query vector (TEI, local)
          - chat/pg.Repository.Search(query, KB_MAX_RESULTS) via `embedding <=> $1::vector`
          - KB/embed failures degrade gracefully to no context
      → BuildMessages(system + RAG context, memory history, chatInput)
      → llm.Client.Complete(..., jsonMode=false)
      → memory.Append(sessionID, chatInput, output)
      → relay {output}, or 502/504 on upstream failure
```

History is stored per `sessionId` in an in-memory store bounded to the last `CHAT_HISTORY_MAX` messages (default 20) with `CHAT_HISTORY_TTL` seconds (default 1800) of inactivity. No chat text is ever persisted.

### Nexxa-Match

```
POST /api/v1/nexxa/match {sessionId?, jawaban_1..8}
  → middleware.RateLimit → match.Handler.NexxaMatch: MaxBytesReader + JSON decode
    → match.Service.NexxaMatch: require exactly 8 answers, each non-empty and ≤500 chars
      → prompt.MatchMessages(normalized)      (system + numbered answers)
      → llm.Client.Complete(messages, jsonMode=true)   → response_format: json_object
      → content.NormalizeNexxaOutput(raw)      (enums, percentages rescaled to 100)
      → relay {nama_jurusan, alasan, persentase_*}, or 502/504 on upstream failure
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

### CV Review

```
POST /api/v1/nexxa/cv-review {cv_text, word_count, page_count}
  → middleware.Auth + middleware.RateLimit → cvreview.Handler.CvReview: MaxBytesReader(1MB) + JSON decode
    → cvreview.Service.CvReview: trim cv_text + require non-empty + ≤50,000 chars; counts ≥ 0
      → prompt.CvReviewMessages(cv_text, word_count, page_count)
      → llm.Client.Complete(messages, jsonMode=true)   → response_format: json_object
      → content.NormalizeCvOutput(raw) → 200 {audit_summary, metrics, ...}
        or 422 on uninterpretable AI output / 502-504 on upstream failure
```

### CV Validate Input and CV Normalize Output

The cv-review validate/normalize transforms behave exactly like the match ones, applied to `{cv_text, word_count?, page_count?}` and the CV audit schema: enums (`ats_status`, `priority`, `category`), score clamping to [0,100], array caps (recommendations ≤ 10, grammar_issues ≤ 8, strengths/key lists ≤ 6), sequential id renumbering, and `has_example` derived from `before_text`/`after_text`. Backend-computed fields (`word_count`, `page_count`, …) are dropped.

The service propagates the request context into the upstream call, so a browser disconnect cancels the LLM completion mid-flight. The client times out via `LLM_TIMEOUT` (default 115s); the server `WriteTimeout` is 120s so slow LLM responses are not cut off.

## Knowledge base (RAG)

- KB content lives as markdown under `KB_DIR` (default `kb/`). Each file is chunked into ~800-char chunks (heading-aware, ~100-char overlap).
- Chunks are embedded with the local TEI model and stored in `kb_chunks.embedding vector(384)` (HNSW `vector_cosine_ops` index) via migration `008`.
- `cmd/kbseed` re-indexes the KB. On server startup, if `KB_AUTO_SEED=true` and the table is empty, seeding runs in the background.
- Retrieval uses cosine distance (`<=>`) and returns up to `KB_MAX_RESULTS` chunks. Embedding or search failures never break chat — they degrade to ungrounded replies.

## Rules

- Input validation is business logic in the service, not the handler.
- `chatInput` is trimmed before length checks; `sessionId` is auto-generated (32 hex chars) when absent.
- Successful and failed chat requests record only message length, an optional topic (max 80 chars), success, and a SHA-256 session hash; raw chat text is never stored. Chat history lives only in the in-memory per-session store.
- Nexxa-Match records success, recommended major, percentages, and a SHA-256 session hash; raw answers and reasons are never stored.
- Nexxa requires exactly 8 answers; answers are trimmed and forwarded normalized.
- The four stateless transforms are pure — no DB, no upstream calls — and complete in well under 500ms.
- `validate-input` never truncates silently: oversized fields are rejected with `400` so the frontend can message the user.
- `normalize-output` returns `422` for unparseable model output; it never leaks raw model output into logs — student answers and raw model text are logged only as SHA-256 prefixes + lengths.
- `normalize-output` percentages always sum to exactly 100 after rescaling; a zero/missing total is a `422` (never guessed).
- Upstream HTTP errors map to `502 Bad Gateway`; timeouts to `504 Gateway Timeout`. Neither leaks upstream response bodies.
- Rate limiting is a token bucket keyed by client IP with no background goroutine (opportunistic cleanup only).
- CV review is stateless: nothing is persisted. `word_count`/`page_count` are backend-computed context for the model, never recalculated or echoed back.
- LLM request payloads are constructed only from validated/sanitized input, models output only JSON when `jsonMode=true`, and both match and CV system prompts embed an injection guard; the backend additionally strips HTML and flags injection patterns in logs.
- `cv_text` is capped at 50,000 chars; handlers allow 1MB bodies so raw CV text is never silently truncated.
- `LLM_BASE_URL` and `LLM_API_KEY` configure a generic OpenAI-compatible endpoint; embed requests go to `LLM_EMBED_BASE_URL` (default `http://localhost:8081/v1`, the TEI container).
- Vector parameters are sent as `$N::vector` string literals — the pool runs in simple-protocol mode, so no pgvector type registration is required.

## cURL examples

```bash
# Chatbot (RAG + memory)
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

# CV Review (auth + rate-limited)
curl -X POST http://localhost:8080/api/v1/nexxa/cv-review \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <token>' \
  -d '{"cv_text":"Nama saya Budi, lulusan SMK dengan pengalaman magang.","word_count":12,"page_count":1}'

# Re-index the knowledge base
go run ./cmd/kbseed --dir kb
```