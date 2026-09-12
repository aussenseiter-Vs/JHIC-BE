---
name: System Design
relation: index.md → core/
description: Architecture overview and system design documentation
type: Enforce
---

# System Design

## Project architecture

This is a **Go** backend service with a **domain-based** structure. The architecture uses a clean layered pattern where each business domain is encapsulated in its own package under `internal/domain/`.

## Domain architecture

```
internal/domain/{domainName}/
├── entity.go          — Domain struct and value objects (e.g., Session)
├── repository.go      — Repository interface(s) for testability
├── service.go         — Business logic
├── handler.go         — HTTP handlers (transport layer)
└── pg/                — Postgres implementation using pgx
```

### Data flow

```
HTTP request
  → handler.go (parse request, validate input, call service)
    → service.go (business logic, call repository interface)
      → pg/ (raw SQL via pgx)
        → PostgreSQL
```

Each layer depends on an **interface** (defined in `repository.go`), so services are unit-testable without a real database.

## Project layout

```
cmd/server/
├── main.go                  — Entry point: config → DB → migrate → wire DI → serve
└── migrations/              — SQL migration files

internal/
├── domain/                  — Business domains
│   ├── auth/
│   ├── user/
│   ├── berita/
│   └── ai/                  — AI (LLM completions + RAG knowledge base)
├── infrastructure/          — Shared infrastructure
│   ├── database/            — pgxpool connection, migration runner
│   ├── middleware/          — CORS, logging, auth, role, rate limit
│   ├── llm/                 — OpenAI-compatible chat completions + embeddings client
│   ├── knowledge/           — markdown chunker + KB seeder (pgvector)
│   ├── response/            — JSON response helpers
│   └── storage/             — S3-compatible storage (Backblaze B2 / MinIO)
├── pkg/id/                  — Snowflake ID generator (stdlib only)
└── router.go                — Route registration + middleware chain

config/
└── config.go                — Env-based config loader
```

## Entry point flow (`cmd/server/main.go`)

1. Load config from environment variables
2. Connect to Postgres via pgxpool
3. Run pending SQL migrations
4. Instantiate repositories → services → handlers (manual DI)
5. Register routes on `http.ServeMux`
6. Instantiate middleware (auth token validator, role checker)
7. Apply middleware chain (logger → CORS — globally; auth + role applied per-route on protected endpoints). CORS reflects the request `Origin` when it matches `CORS_ALLOWED_ORIGINS` (comma-separated env list, `*` default), sends `Access-Control-Allow-Credentials: true` for reflected origins, and short-circuits preflight `OPTIONS` with `204` (or `403` for disallowed origins).
8. Start server with graceful shutdown on SIGINT/SIGTERM

## Key principles

- **No framework.** Uses Go stdlib `net/http` with Go 1.22+ `http.ServeMux` (supports method-based routing like `GET /users/{id}`).
- **Raw SQL.** All database queries are hand-written SQL via pgx. No ORM.
- **Domain isolation.** Each domain is a self-contained package. No cross-domain entity imports.
- **Interface-based repos.** `repository.go` defines the contract; the `pg/` subpackage implements it. Swap implementations for testing.
- **Manual DI.** Dependencies are wired explicitly in `main.go`. No reflection, no DI framework.
- **Opaque bearer tokens.** Sessions stored in a `sessions` table. No JWT dependency. Instant revocation.
