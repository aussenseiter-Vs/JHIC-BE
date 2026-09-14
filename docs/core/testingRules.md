---
name: Testing Rules
relation: index.md → core/
description: Mandatory testing strategy — unit, component integration, and E2E API test rules
type: Enforce
---

# Testing Rules

## Testing tiers

| Tier | Target | External deps | Run command |
|---|---|---|---|
| Unit | Pure business logic (`service.go`) | Mocked | `go test ./...` |
| Component Integration | Structural components (`pg/` adapters, storage) | Real, containerized | `go test -tags=integration ./...` |
| E2E API | Full HTTP transaction through the real router | Real, containerized | `go test -tags=e2e ./...` |

All three tiers are mandatory. A feature is not complete until it has coverage at the tiers where its behavior lives: business rules in unit tests, storage/DB interaction in component tests, and the end-to-end workflow in E2E tests.

## 1. Unit Tests (pure business logic)

Focus entirely on pure business logic. Test a single function in isolation by mocking external network requests and database connections.

- Target `service.go` only. Never spin up a database, never make network calls, never construct the real handler.
- Mocks are generated with **mockery** and use **testify** for assertions. They implement the domain `Repository` interfaces defined in `repository.go`.
- File conventions: `service_test.go` in the domain package, generated mocks under `mocks/`.
- Use table-driven tests. Cover success paths and every error path the service can produce (not found, forbidden: not the author, duplicate email, invalid input).
- Verify mock call expectations (`assert`/`require` on called-with arguments) and return values.
- Unit tests carry **no build tag** — they are the default suite.

```go
func TestService_Update(t *testing.T) {
    mockRepo := mocks.NewRepository(t)
    mockRepo.On("ByID", mock.Anything, "berita-1").Return(&Berita{ID: "berita-1", AuthorID: "user-1"}, nil)
    mockRepo.On("Update", mock.Anything, mock.MatchedBy(func(b *Berita) bool { return b.Title == "New" })).Return(nil)

    svc := NewService(mockRepo)
    got, err := svc.Update(context.Background(), "berita-1", "user-1", "New", "content")
    require.NoError(t, err)
    require.Equal(t, "New", got.Title)
    mockRepo.AssertExpectations(t)
}
```

## 2. Component Integration Tests

Validate how code interacts with structural components. This includes tracking active database connections, third-party services, cache layers, and message brokers.

- The `pg/` adapter of each domain is tested against a **real Postgres** container; the storage `Client` is tested against a **real directory** (filesystem round-trip).
- Every structural component gets the same treatment: containerize the real thing, exercise the real behavior, assert results, assert resource cleanup.
- **Track active database connections.** Assert `pool.Stat()` (`TotalConns`, `AcquiredConns`) before and after each test — connection counts must return to baseline. A rising `AcquiredConns` after tests is a leak and fails the suite.
- Verify third-party service round-trips end to end: `Upload` → object exists → `Get` → bytes are fetchable → `Delete` → object gone.
- Cache layers and message brokers, when introduced, follow the same shape: real component, containerized, behavior asserted, connections/goroutines cleaned up via `t.Cleanup`.
- Every file carries the build tag `//go:build integration`.

```go
//go:build integration

func TestRepository_Create(t *testing.T) {
    pool := startPostgres(t) // t.Cleanup terminates container
    before := pool.Stat().AcquiredConns()
    repo := pg.NewRepository(pool)
    // ... exercise, assert
    require.Equal(t, before, pool.Stat().AcquiredConns())
}
```

## 3. E2E API Tests

Simulate full transactional workflows. Trigger an HTTP request, allow it to route through the logic layer, and confirm the permanent state changes inside the database.

- Build the **real router** (`internal.NewRouter`) with real handlers, services, and PG repositories, and serve it with `httptest.NewServer`.
- Requests flow handler → service → repository → database exactly as in production. No mocks.
- **Confirm permanent state changes** by querying the database directly after each request: the row exists with the right fields, `UPDATE` persisted, `DELETE` removed the row, logout invalidated the session, image upload left an object in storage.
- Canonical workflows to cover:
  - `register` → user row exists in DB → returned token works on an authed endpoint
  - `login` → session row exists → `logout` → same token is rejected
  - berita `create`/`list`/`get`/`update`/`delete` → assert DB rows at each step and that `delete` also removed the stored image
  - `upload image` → object present on disk, `image_url` persisted, the proxy endpoint serves the bytes
- Every file carries the build tag `//go:build e2e`.

```go
//go:build e2e

func TestE2E_BeritaLifecycle(t *testing.T) {
    srv, pool, store := startE2E(t) // real router + Postgres + local-disk storage
    token := registerAndLogin(t, srv.URL) // returns usable Bearer token
    // POST /api/v1/berita ...
    // assert via direct DB query that the berita row exists with author_id set
    // DELETE /api/v1/berita/{id} ...
    // assert via direct DB query that the row is gone
}
```

## 4. Test infrastructure (Testcontainers)

- Component and E2E tests spin up a throwaway **Postgres** container with `testcontainers-go`; image storage uses a throwaway temp directory. One shared setup helper per package (`startPostgres`, `startE2E`) keeps container lifecycle in `t.Cleanup`.
- Apply the SQL migrations from `cmd/server/migrations/` on container start so the schema matches production.
- Truncate all tables between tests (or use a fresh schema per test) so tests never depend on each other.
- Tests are **fully hermetic** — they never connect to the real production database or any external service. Running a test must be incapable of touching production data.

## 5. Test hygiene rules

- Run the default suite with the race detector: `go test -race ./...`.
- No sleeps or polling hacks. Use proper synchronization (`sync.WaitGroup`, channels, retry loops with timeout) when waiting on async behavior.
- Never assert on `time.Now()`-dependent fields directly; inject or freeze time where a test asserts timestamps.
- Use `t.Cleanup` for every resource (containers, pools, servers). Use `t.Helper()` on shared helpers so failures point at the caller.
- Do not run tests in parallel when they mutate shared state (the same DB schema or package-level globals).

## 6. Dependencies

Test-only dependencies are added to `go.mod` only when the corresponding test files are introduced:

- `github.com/stretchr/testify` — assertions (`assert`, `require`)
- `github.com/stretchr/testify/mock` — mock objects for unit tests
- `github.com/vektra/mockery/v2` — mock generation (CLI, not a runtime dep)
- `github.com/testcontainers/testcontainers-go` — Postgres containers for component and E2E tiers

None of these appear in the production dependency graph of `cmd/server`.
