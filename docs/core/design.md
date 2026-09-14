---
name: Design Decisions
relation: index.md → core/
description: Design decisions and rationale to prevent agent drift
type: Enforce
---

# Design Decisions

## No framework (2026-07-20)

**Decision:** Use Go stdlib `net/http` with Go 1.22+ `http.ServeMux`. No Gin, Echo, Chi, or any third-party HTTP framework.

**Rationale:** Go's stdlib is production-grade. Go 1.22 added method-based routing (`GET /users/{id}`, `POST /users`) and path parameters (`r.PathValue("id")`), eliminating the two main reasons teams reached for third-party routers. Adding a framework introduces dependency risk, abstraction leakage, and framework-specific magic that fights Go's explicit nature.

## Raw SQL over ORM (2026-07-20)

**Decision:** Use `pgx` directly with hand-written SQL. No GORM, ent, or sqlboiler.

**Rationale:** ORMs in Go introduce reflection overhead, surprising behavior (silent zero-value issues in GORM), and obscure the actual queries hitting the database. Complex queries inevitably require raw SQL anyway, creating a hybrid that's worse than either pure approach. Direct pgx gives full control over query plans, native Postgres type support (JSONB, arrays, UUID), and predictable performance.

## Domain isolation (2026-07-20)

**Decision:** Each domain is a standalone Go package under `internal/domain/`. Domains do not import each other's entities.

**Rationale:** Cross-domain imports create tight coupling and circular dependency issues. If a domain needs data from another domain, it goes through the service layer (e.g., `user.Service` called by a higher-level orchestrator), not by importing user entities into the auth package.

## Manual dependency injection (2026-07-20)

**Decision:** Wire all dependencies in `cmd/server/main.go`. No DI framework, no reflection, no `wire`.

**Rationale:** Explicit wiring is the Go way — it's traceable, debuggable, and requires no code generation. The tradeoff is more lines in `main.go` as the project grows, but this is acceptable for a monolith with a handful of domains. If the project grows to dozens of domains, consider a `wire`-based approach, but start simple.

## Repository interfaces for testability (2026-07-20)

**Decision:** Every domain has a `repository.go` file defining an interface, and a `pg/` subpackage implementing it with pgx (types drop the `PG` suffix inside the adapter package).

**Rationale:** The interface lets services be unit-tested with mock repositories without spinning up a database. It also makes it trivial to swap implementations (e.g., adding Redis caching via a decorated repository). The `pg/` adapter package keeps the domain root tidy as the pgx implementation and its integration tests grow.

## Custom migration runner (2026-07-20)

**Decision:** Use a simple migration runner in `internal/infrastructure/database/migrate.go` that reads `.sql` files and tracks applied versions in a `schema_migrations` table.

**Rationale:** `golang-migrate/migrate` requires driver registration and database/sql compatibility layers. A custom runner with `embed` or filesystem reads is ~60 lines, has zero extra dependencies, and is fully transparent. Down migrations are handled manually via SQL scripts when needed.

## In-place CREATE migrations for pre-production (2026-07-31)

**Decision:** Never create an ALTER TABLE migration file for environments that have not yet run the original CREATE migration. Instead, edit the original `00X_*.sql` file in place and change every dependent file in the same commit — pg adapter queries, entity structs, integration tests, E2E tests, docs. New numbered migration files are reserved for production (where the original migration already ran) and for data backfills.

**Rationale:** The runner records applied versions in `schema_migrations` keyed by version number, so an edited file is applied only on databases that have never seen that version. Pre-production environments (local dev, CI, Testcontainers) rebuild the schema from scratch constantly, so editing the CREATE keeps a single authoritative migration per schema change, eliminates "CREATE followed immediately by ALTER" noise, and prevents the code, tests, and migration set from drifting apart. Production has the old file recorded as applied, so a new numbered migration is required there.

## Snowflake IDs (2026-07-31)

**Decision:** Generate snowflake IDs client-side in Go (`internal/pkg/id/id.go`), stored as `BIGINT` in Postgres and serialized as decimal strings in JSON. The `id.ID` named type (int64) is used across entities, repositories, services, handlers, and middleware instead of `string`.

**Rationale:** UUIDs are opaque, unorderable blobs (36 chars, no sort semantics, no embedded time), while snowflakes are 64-bit ints that are sortable, index-friendly (BIGINT beats TEXT), and embed a millisecond timestamp (41 bits since `2024-01-01` UTC) plus a node ID (10 bits, from `SNOWFLAKE_NODE_ID`, default 0) and a per-millisecond sequence (12 bits). The `sync.Mutex`-guarded generator guarantees monotonicity under concurrency, including clock-skew sleeps. Client-side generation (the same property that motivated the original UUID decision) lets services create entities and return them immediately without a round-trip to the database. IDs are marshaled to decimal strings so JavaScript clients never lose precision on values above 2^53. `created_at` is still kept on every table: it remains queryable and DB-defaulted at insert time, independent of the client-generated ID timestamp.

## Local-disk storage (2026-09-14)

**Decision:** Store uploaded berita images as plain files under a local directory (`STORAGE_DIR`, default `data`) using a filesystem `storage.Client`, replacing the previous S3-compatible (Backblaze B2 / MinIO) object store. Images are served to clients through a read-through proxy endpoint (`GET /api/v1/berita/images/{key}`) that streams the file from disk per request, so served URLs never expire.

**Rationale:** The images are low-volume and only ever consumed by this API, so an external object store added operational overhead (credentials, region, bucket, S3 SDK) without benefit; a local directory plus the existing read-through proxy keeps the API self-contained. The `storage.Client` interface (`Upload`, `Get`, `Delete`) is unchanged, so the storage layer stays swappable if a real object store is ever needed. Files are written under `berita/{beritaID}/{uuid}.{ext}` keys, path traversal is rejected at resolve time, and content type is re-detected from the file bytes on read so the proxy can set a correct `Content-Type`. (Presigned URLs were the initial approach; they were retired because a URL baked into stored content silently dies after the 24h TTL.)

## Role-based access control (2026-07-20)

**Decision:** Implement RBAC as middleware (`internal/infrastructure/middleware/role.go`) that checks the caller's role against an allowed list. Roles are stored in the `users` table.

**Rationale:** Middleware-level RBAC is declarative — a handler declares which roles are allowed when registering its routes (e.g., `RequireRole("jurnal")`). Role lookup uses the `UserRepository.ByID` interface, keeping the middleware agnostic of the user domain implementation. Supported roles: `admin`, `jurnal`, `guru`, `user`.

## Image upload with server-side MIME validation (2026-07-20)

**Decision:** Validate image MIME types server-side using `net/http.DetectContentType`, enforce a 5 MB max upload size via `http.MaxBytesReader`, and store images under `berita/{beritaID}/{uuid}.{ext}` paths in local storage.

**Rationale:** Client-side MIME checks are trivially bypassed. Server-side detection using the first 512 bytes (`http.DetectContentType`) matches what browsers send and prevents non-image uploads. MaxBytesReader limits memory usage and prevents abuse. The key prefix per berita groups related images together in the bucket listing.
