# AGENTS.md

Guidance for agentic coding agents working in **CloudKaho**.

## Repo facts (read this first)
- Language: **Go** (module: `github.com/yurin-kami/CloudKaho`, `go 1.26.1` in `go.mod`).
- Frameworks: **Gin** (HTTP), **GORM** (MySQL), Redis client; OpenAPI spec in `openapi.yaml`.
- Local deps: `docker-compose.yml` provides **MySQL** + **Redis**.
- Config: `config/config.go` reads **`config.toml` from the current working directory**.
  - Do **not** commit secrets (DB passwords/JWT/S3 keys). Prefer local-only config.

## Cursor / Copilot rules
- Cursor rules: **not found** (no `.cursor/rules/` and no `.cursorrules`).
- GitHub Copilot instructions: **not found** (no `.github/copilot-instructions.md`).

## OpenCode / agent philosophy (repo-specific)
This repo contains OpenCode guidance:
- Philosophy gate: `.opencode/tools/philosophy.md`
- Skills: `.opencode/skills/code-philosophy/SKILL.md`, `.opencode/skills/frontend-philosophy/SKILL.md`
If you are operating as an agent, **load `code-philosophy` for backend changes** (or `frontend-philosophy` for UI) before implementing.

## Canonical commands

### Run (local)
- The OpenAPI spec (`openapi.yaml`) declares the server as `http://localhost:8080`.

### Build
- Build all packages:
  - `go build ./...`

### Test
- Run all tests (currently there are no `*_test.go` files; this mainly compiles):
  - `go test ./...`

### Run a single test (when tests exist)
- Single package, single test:
  - `go test ./path/to/pkg -run '^TestName$' -count=1 -v`
- Single test across all packages:
  - `go test ./... -run '^TestName$' -count=1 -v`
- Subtest:
  - `go test ./path/to/pkg -run '^TestName$/^SubtestName$' -count=1 -v`
- Benchmark:
  - `go test ./path/to/pkg -run '^$' -bench '^BenchmarkName$' -count=1`

### Lint / static checks
- Vet:
  - `go vet ./...`

### Format
- Check formatting (prints files that need formatting):
  - `gofmt -l .`
- Auto-format:
  - `gofmt -w .`

> Tip: prefer running `gofmt -w .` before `go test ./...` to avoid format-only diffs later.

## Local environment
- Start dependencies:
  - `docker compose up -d`
- `docker-compose.yml` exposes the default ports:
  - MySQL: `3306:3306`
  - Redis: `6379:6379`
- MySQL init scripts (if present) are mounted from `./db` into the MySQL container init directory.
  - This repo references `db/seed.sql`.

### Config (local-only)
- `config/config.go` loads `config.toml` from the **current working directory**.
  - When running any code that calls `config.MustLoad()`, ensure your working directory is the repo root (or wherever your `config.toml` lives).
- Treat `config.toml` as **local environment state**.
  - Do not commit secrets (DB passwords/JWT/S3 keys).
  - The repo’s `.gitignore` lists `config.toml`.

## Project structure (high-level)
- `cmd/` — entrypoints.
- `config/` — config singleton + TOML loading.
- `internal/routes/` — Gin route registration.
- `internal/middleware/` — JWT auth middleware.
- `internal/user/`, `internal/file/` — HTTP handlers.
- `service/` — business logic + DB/Redis/S3 operations + tx helpers.
- `models/` — GORM models.

## Code style (match existing patterns)

### Imports
- Let **gofmt** handle import grouping/sorting.
- Prefer standard-library → third-party → local module groups.

### Formatting
- Always run `gofmt` on touched Go files.
- Prefer guard clauses / early returns to avoid deep nesting.

### Types & boundaries
- Keep parsing/validation at boundaries (HTTP handlers, config loading).
- Inside service logic, assume normalized inputs; return explicit errors for domain failures.

### Naming
- Packages: short, lower-case (`service`, `models`, `routes`, `middleware`).
- Request/response structs in handlers: `XxxRequest` (existing pattern).
- Booleans: `is/has/can/should` prefixes.

### Error handling
- Use sentinel/domain errors in `service/errors.go` (`ErrNotFound`, `ErrConflict`, `ErrInvalid`, `ErrForbidden`).
- Check expected errors with `errors.Is(err, service.ErrXxx)` and map them to HTTP responses.
- Add context when returning errors from service/data layers.
- Do not swallow errors; fail fast and return a clear message.

#### HTTP status mapping (recommended)
When service code returns sentinel errors, handlers should convert them to consistent HTTP status codes:
- `service.ErrInvalid` → `400 Bad Request`
- `service.ErrForbidden` → `403 Forbidden`
- `service.ErrNotFound` → `404 Not Found`
- `service.ErrConflict` → `409 Conflict`
- Unknown/unexpected errors → `500 Internal Server Error` (include a safe `details` message when helpful)

### HTTP handlers (Gin)
- Use `context.WithTimeout(c.Request.Context(), ...)` for downstream calls; `defer cancel()`.
- Auth: middleware sets `userID` in Gin context (`c.Set("userID", ...)`).
- Responses often use a simple envelope: `{"code":"0"}` success / `{"code":"1","message":...}` failure.
  - Keep responses consistent within the file/module you touch.

#### Response consistency
- Prefer matching `openapi.yaml` response shapes:
  - Success: `{"code":"0", ...}`
  - Error: `{"code":"1","error":<string>,"details":<string|null>}` (see `components.schemas.ErrorResponse`).
- Avoid mixing multiple error envelope styles in the same module (some older handlers may return only `{"error": ...}` in a few places).
- When returning `details`, ensure it does **not** leak secrets (tokens, DSNs, S3 keys, etc.).

### DB & transactions (GORM)
- Prefer `db.WithContext(ctx)`.
- For MySQL transactional work, follow existing patterns:
  - Manual `Begin/Commit/Rollback` with `defer` recovery, or
  - `WithTxRetry(ctx, db, maxRetries, fn)` for deadlock/lock-timeout retry (`service/txRetry.go`).
- When doing read-modify-write, consider `SELECT ... FOR UPDATE` via `clause.Locking{Strength:"UPDATE"}` as used in `service/fileService.go`.

### Logging
- Current code uses stdlib `log` / `log.Printf`.
- Do not log secrets (JWT, passwords, DSNs, S3 keys) or sensitive payloads.

### Security hygiene (especially in middleware/handlers)
- JWT auth uses the `Authorization: Bearer <token>` header (see `internal/middleware/auth.go`).
  - Do **not** log the `Authorization` header or raw tokens.
- Treat S3-related identifiers as sensitive operational details.
  - Do not log access keys/secret keys.
  - Avoid logging full presigned URLs or request headers used to generate them.
- Use context timeouts for DB/Redis/S3 calls initiated from HTTP handlers.
  - Prefer threading the request context (`c.Request.Context()`) into service methods.

## Agent workflow expectations
- Make minimal, focused diffs.
- Prefer adding tests for bug fixes/features when feasible.
- Before finalizing, run: `gofmt -w . && go vet ./... && go test ./...`.

## Common pitfalls (repo-specific)
- **Local-only / ignored files:** the repo’s `.gitignore` includes `config.toml`, `bin/`, `db/`, `openapi.yaml`, and `docker-compose.yml`.
  - Avoid committing binaries or environment-specific artifacts.
  - If you create local helper scripts/config, prefer keeping them untracked.
