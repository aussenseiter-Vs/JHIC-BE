---
name: Domain Creation Rules
relation: index.md → modules/
description: Enforceable rules for creating and structuring Go backend domains
type: Enforce
---

# Domain Creation Rules

## 1. Domain structure

Every business domain is a **top-level package** under `internal/domain/`. Each domain must include this exact file set:

```
internal/domain/{domainName}/
├── entity.go          — Domain struct (no JSON tags on internal fields)
├── repository.go      — Repository interface (methods the service needs)
├── service.go         — Business logic (calls repository interface)
├── handler.go         — HTTP handlers + route registration
├── mocks/             — Mockery-generated test mocks (test-only)
└── pg/                — PostgreSQL adapter package
    ├── {adapter}.go   — pgx implementation(s) of the repository
    ├── helper_test.go — shared container setup for integration tests
    └── repository_integration_test.go — integration tests (build tag `integration`)
```

### Optional files

- `types.go` — Request/response types if they grow beyond the handler file.
- `errors.go` — Sentinel errors for the domain.

### Non-DB domains (e.g. `nexxa`)

Domains with no persistence swap the `pg/` + `repository.go` contract for an external-client contract:

```
internal/domain/{domainName}/
├── client.go          — Client interface the service depends on
├── entity.go          — Domain structs
├── errors.go          — Sentinel errors (validation + upstream failures)
├── service.go         — Business logic (validation, calls client interface)
├── handler.go         — HTTP handlers + route registration
├── mocks/             — Mockery-generated test mocks
```

When a non-DB domain grows beyond a single sub-domain (more than 4 endpoints or clear logical boundaries), it must be split into sub-packages:

```
internal/domain/{domainName}/
├── client.go          — Shared client interface(s)
├── entity.go          — Shared entity types (only what sub-packages need)
├── errors.go          — Shared sentinel errors (upstream failures)
├── mocks/             — Mockery-generated test mocks for shared interfaces
├── {subdomain1}/      — First sub-domain
│   ├── entity.go      — Sub-domain-specific types
│   ├── errors.go      — Sub-domain-specific sentinel errors
│   ├── service.go     — Business logic
│   ├── handler.go     — HTTP handlers + Register
│   └── *_test.go      — Unit tests
├── {subdomain2}/      — Second sub-domain
│   └── ...
└── {subdomainN}/      — Additional sub-domains
    └── ...
```

Rules for sub-domains:
- The parent package must **not** import any sub-domain package (avoids Go import cycles). Sub-domains import the parent for shared types.
- Each sub-domain has its own `entity.go`, `errors.go`, `service.go`, `handler.go`, and test files.
- Each sub-domain implements its own `Register(mux *http.ServeMux, ...)` method.
- Route registration for all sub-domains happens in `internal/router.go` — import each sub-domain package and call its `Register`.
- DI wiring in `cmd/server/main.go` constructs each sub-domain's service and handler independently.
- Shared mocks (for the parent's shared interface) live in the parent's `mocks/` directory.

The real implementation lives in `internal/infrastructure/{service}/` (e.g. `internal/infrastructure/n8n/`) and imports the domain package for its types. Services wrap upstream failures in the domain's sentinel errors so handlers can map them to status codes (e.g. 502/504) without leaking internal detail.

## 2. File contracts

### entity.go

Define the domain struct. Use `json:"name,omitempty"` tags for optional fields.

```go
type User struct {
    ID        id.ID     `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### repository.go

Define the interface the service layer depends on. Keep it minimal — only methods the service actually calls.

```go
type Repository interface {
    Create(ctx context.Context, u *User) error
    ByID(ctx context.Context, id string) (*User, error)
    List(ctx context.Context) ([]User, error)
    Update(ctx context.Context, u *User) error
    Delete(ctx context.Context, id string) error
}
```

### pg/ (PostgreSQL adapter)

Implement the repository with `pgx` in a `pg/` subpackage. Name the file after the adapter it implements: `repository.go` for a `Repository`, `users.go` / `sessions.go` for `UsersRepository` / `SessionsRepository`. Types drop the `PG` suffix — the package already says it.

```go
type Repository struct {
    pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
    return &Repository{pool: pool}
}
```

- Use `pgx.ErrNoRows` for missing records → return `nil, nil`.
- Wrap all errors with context: `fmt.Errorf("method name: %w", err)`.
- Consumers wire it as `pg.NewRepository(pool)`. When a file imports more than one domain's `pg` package, alias the imports (e.g. `userpg`, `beritapg`).

The `pg/` folder must always include these three files for testing:

| File | Purpose |
|---|---|
| `{adapter}.go` | pgx implementation of the repository |
| `helper_test.go` | Shared container setup (Postgres) for integration tests |
| `repository_integration_test.go` | Integration tests (build tag `integration`) |

This structure ensures every DB-backed domain has a consistent testing path: unit tests use the `mocks/` package, integration tests use `pg/helper_test.go` + `pg/repository_integration_test.go`.

### service.go

Business logic layer. Constructor takes the `Repository` interface.

```go
type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}
```

- Use `internal/pkg/id.New()` for ID generation. IDs are `id.ID` (int64 snowflakes); pass them as `id.ID`, never as `string`. JSON serialization handles string encoding automatically.
- Timestamps use `time.Now().UTC()`.
- Validate business rules here (e.g., duplicate email check).

### handler.go

HTTP transport layer. Constructor takes `*Service`.

```go
type Handler struct {
    svc *Service
}

func NewHandler(svc *Service) *Handler {
    return &Handler{svc: svc}
}
```

Must implement a `Register` method. For public or auth-only domains, accept only the mux:

```go
func (h *Handler) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/v1/users", h.List)
    mux.HandleFunc("GET /api/v1/users/{id}", h.Get)
    mux.HandleFunc("PUT /api/v1/users/{id}", h.Update)
    mux.HandleFunc("DELETE /api/v1/users/{id}", h.Delete)
}
```

For domains that require auth + role middleware on all routes, accept the middleware functions as parameters:

```go
func (h *Handler) Register(mux *http.ServeMux, authMw func(http.Handler) http.Handler, roleMw func(http.Handler) http.Handler) {
    mux.Handle("POST /api/v1/berita", authMw(roleMw(http.HandlerFunc(h.Create))))
    mux.Handle("GET /api/v1/berita", authMw(roleMw(http.HandlerFunc(h.List))))
    mux.Handle("GET /api/v1/berita/{id}", authMw(roleMw(http.HandlerFunc(h.Get))))
    mux.Handle("PUT /api/v1/berita/{id}", authMw(roleMw(http.HandlerFunc(h.Update))))
    mux.Handle("DELETE /api/v1/berita/{id}", authMw(roleMw(http.HandlerFunc(h.Delete))))
    mux.Handle("POST /api/v1/berita/{id}/image", authMw(roleMw(http.HandlerFunc(h.UploadImage))))
}
```

- Parse request bodies with `json.NewDecoder`.
- Validate input at the handler level (required fields, format).
- Use `response.JSON` and `response.Error` helpers for all responses.
- Path parameters use `r.PathValue("id")`.

## 3. Route registration

Each handler registers its own routes via `Register`. The central `internal/router.go` calls each handler's `Register`. For domains with sub-domains, each sub-domain registers independently:

```go
func NewRouter(ah *auth.Handler, uh *user.Handler, bh *berita.Handler, pklHnd *pkl.Handler, chatHnd *chat.Handler, matchHnd *match.Handler, authMw func(http.Handler) http.Handler, roleMw func(http.Handler) http.Handler, roleCheck middleware.RoleChecker) http.Handler {
    mux := http.NewServeMux()
    ah.Register(mux)
    uh.Register(mux, authMw, roleCheck)
    bh.Register(mux, authMw, roleMw)
    pklHnd.Register(mux, authMw, roleCheck)
    chatHnd.Register(mux)
    matchHnd.Register(mux)
    // ... global middleware chain
}
```

## 4. Wiring (DI)

All dependencies are wired in `cmd/server/main.go`:

```go
repo := domainpg.NewRepository(pool)
svc  := domain.NewService(repo)
hnd  := domain.NewHandler(svc)
```

For domains with sub-domains, each sub-domain is wired independently:

```go
svc1 := subdomain1.NewService(sharedClient)
hnd1 := subdomain1.NewHandler(svc1, limit)
svc2 := subdomain2.NewService(sharedClient)
hnd2 := subdomain2.NewHandler(svc2, limit)
```

Order: repository depends on pool → service depends on repository → handler depends on service.

## 5. Naming conventions

| Artifact | Pattern | Example |
|---|---|---|---|
| Directory | `internal/domain/{name}` | `internal/domain/nexxa` |
| Sub-domain directory | `internal/domain/{name}/{subdomain}` | `internal/domain/nexxa/chat` |
| Entity struct | PascalCase singular | `type User struct` |
| Repository interface | `Repository` | `type Repository interface` |
| PG implementation (in `pg/`) | `Repository` / `UsersRepository` | `type Repository struct` in `internal/domain/{name}/pg/` |
| Service | `Service` | `type Service struct` |
| Handler | `Handler` | `type Handler struct` |
| Constructor | `New{Type}` | `func NewRepository(pool *pgxpool.Pool) *Repository` |
| Handler methods | `List`, `Create`, `Get`, `Update`, `Delete` | `func (h *Handler) List` |
| Input types | `{Action}{Domain}Input` | `CreateUserInput` |

## 7. Sub-domain structure

When a domain has more than 4 endpoints or clear logical boundaries (e.g. chat vs. match), it must be split into sub-packages under the domain directory. Each sub-domain follows the same file contract as a standalone domain:

```
internal/domain/{domainName}/
├── client.go          — Shared client interface(s) (non-DB domains only)
├── entity.go          — Shared entity types (only what sub-domains need)
├── errors.go          — Shared sentinel errors (upstream failures)
├── mocks/             — Shared mocks for shared interfaces
├── {subdomain1}/      — Sub-domain 1
│   ├── entity.go      — Sub-domain-specific types
│   ├── errors.go      — Sub-domain-specific sentinel errors
│   ├── service.go     — Business logic
│   ├── handler.go     — HTTP handlers + Register
│   ├── mocks/         — Sub-domain-specific mocks (if needed)
│   ├── *_test.go      — Unit tests co-located with source
│   └── helper_test.go — Sub-domain test helpers (if needed)
├── {subdomain2}/      — Sub-domain 2
│   └── ...
└── {subdomainN}/      — Sub-domain N
    └── ...
```

Rules:
- The parent package must **not** import any sub-domain package (avoids Go import cycles).
- Sub-domains import the parent for shared types/interfaces.
- Each sub-domain has its own `Register(mux *http.ServeMux, ...)` method.
- Test files must be co-located with source files in each sub-package (same pattern as standalone domains).
- Each sub-domain may have its own `mocks/` folder for sub-domain-specific mockable interfaces.
- `internal/router.go` imports each sub-domain package directly and calls its `Register`.
- `cmd/server/main.go` wires each sub-domain's service and handler independently.
- For DB-backed sub-domains, each sub-domain may have its own `pg/` folder with `{adapter}.go`, `helper_test.go`, and `repository_integration_test.go` (build tag `integration`).

## 8. No dead code

Don't generate files, exports, or types that nothing consumes. Empty stubs create confusion and maintenance burden. Every exported symbol must have at least one consumer.
