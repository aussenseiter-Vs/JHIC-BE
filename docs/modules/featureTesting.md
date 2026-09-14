---
name: Feature Testing Procedure
relation: index.md → modules/
description: Mandatory step-by-step procedure for testing a new feature or behavior change
type: Enforce
---

# Feature Testing Procedure

## 1. When to use

Follow this procedure whenever you implement a new feature, add or change an endpoint, or modify existing behavior. `docs/core/testingRules.md` governs **all** test code — read it before writing or modifying any test. It defines the three tiers, the mocking toolchain, and the Testcontainers infrastructure this procedure relies on.

## 2. Tier selection

Map the change to the tiers where its behavior lives. Every feature must be covered at those tiers:

| Kind of change | Required tier | Where |
|---|---|---|
| Business rule, validation, authorization logic | Unit | `internal/domain/{domain}/service_test.go` |
| SQL queries, repository behavior | Component integration | `internal/domain/{domain}/pg/repository_integration_test.go` |
| Storage / third-party service interaction | Component integration | `internal/infrastructure/storage/b2_integration_test.go` |
| New or changed route, HTTP workflow | E2E API | `internal/e2e/e2e_test.go` |

A change that spans layers (e.g. a new endpoint that validates, queries, and writes) requires coverage at every tier it touches.

## 3. Step-by-step procedure

1. **Identify affected domains.** Determine which package(s) under `internal/domain/` the feature touches, and whether the router or storage layer is involved.

2. **Regenerate mocks if interfaces changed.** If `repository.go` signatures changed, regenerate the mocks with mockery:

   ```sh
   go run github.com/vektra/mockery/v2@latest --name {InterfaceName} --dir internal/domain/{domain} --output internal/domain/{domain}/mocks --outpkg mocks
   ```

   Update existing mock expectations in `service_test.go` to match the new signatures.

3. **Unit tests.** Extend or add `service_test.go` in the affected domain. Table-driven, covering the success path and **every error path** the service can return (not found, forbidden, duplicate, invalid input, repository failure). Verify call arguments with `require`/`assert` on mock expectations. No DB, no network.

4. **Component integration tests.** If the change touches SQL or a third-party service, extend `pg/repository_integration_test.go` or `b2_integration_test.go`:
   - Use the package's shared container helper (`startPostgres`, or the local-disk storage client) — never start your own.
   - Seed prerequisites directly via SQL (e.g. a user row for FK constraints).
   - Keep the connection-leak assertion: record `pool.Stat().AcquiredConns()` before and assert it returns to baseline after.
   - For storage round-trips: `Upload` → object exists → `PresignGet` → signed URL fetchable → `Delete` → object gone.

5. **Schema changes.** Prefer zero-schema changes. When a new table or column is unavoidable and the target environment has **not** yet run the original migration (anything except production), **edit the original CREATE migration in place** — `cmd/server/migrations/00X_name.sql` — and update every dependent file in the same commit: pg adapter queries, entity structs, integration tests, E2E tests, docs. Do **not** create an ALTER TABLE migration for pre-production. New numbered `00X_*.sql` files are reserved for environments that already ran the original migration (production) and for data backfills. The runner records applied versions in `schema_migrations` keyed by version number, so an in-place edit is skipped on any DB that already applied the old file.

6. **E2E API tests.** Extend or add `TestE2E_*` in `internal/e2e/e2e_test.go`. Drive a real HTTP request through the real router, then **confirm permanent state changes** by querying the database or storage directly (row exists / updated / deleted, session invalidated, object stored / removed). Reuse the existing helpers (`register`, `doJSON`, `uploadImage`, `promoteToJurnal`).

7. **Run the suites.** Run every applicable command (section 4) and fix until green.

8. **Update domain docs.** If the feature changes user-visible behavior or the domain contract, update the corresponding `docs/modules/{domain}/` documentation.

## 4. Run commands

| Tier | Command |
|---|---|
| Unit | `go test -race ./...` |
| Component integration | `go test -tags=integration ./...` |
| E2E API | `go test -tags=e2e ./internal/e2e/` |

Requires a running Docker daemon for the integration and E2E tiers.

## 5. Completion checklist

A feature is not "tested" until all of the following hold:

- [ ] Unit tests cover the success path and every error path the service can return.
- [ ] Component tests exercise real behavior against containerized Postgres / local-disk storage, with the connection-leak baseline assertion.
- [ ] E2E tests assert permanent state changes via direct DB/S3 queries, not just response codes.
- [ ] `go test -race ./...` passes.
- [ ] Mocks regenerated and expectations updated when `repository.go` interfaces changed.
- [ ] No sleeps or polling hacks; no `t.Parallel()` on tests sharing schema/state.
- [ ] Pre-production schema changes edit the original CREATE migration in place with all dependent files updated in the same commit; new numbered `00X_*.sql` files only for production and data backfills.
- [ ] No dead test code — every helper added is used, every test asserts something.
- [ ] Domain docs updated if behavior changed.
