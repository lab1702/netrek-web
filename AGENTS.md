# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: Entry point; serves static files and exposes `/ws`, `/api/teams`, and `/health`.
- `server/`: Game server, bots, and HTTP/WebSocket handlers (most logic and tests).
- `game/`: Core game types, physics, and helpers.
- `static/`: Client assets (HTML/JS) embedded at build time.
- Docker: `Dockerfile`, `docker-compose.yml` (maps `8080:8080`).

## Build, Test, and Development Commands
- Run locally: `go run . -port 8080` → open `http://localhost:8080`.
- Build binaries: `go build ./...`.
- Run tests: `go test ./...` (all) or `go test ./server -run Intercept -v` (filter).
- Format & vet: `go fmt ./...` and `go vet ./...`.
- Docker dev: `docker compose up --build` (health at `/health`).
- Scripts: `./update_docker.sh` (rebuild image), `./watch_docker.sh` (live rebuilds; requires bash).

## Coding Style & Naming Conventions
- Go 1.20+; use `gofmt` defaults (tabs, standard import order).
- Packages are lowercase; files use `snake_case.go`.
- Exported identifiers `CamelCase` (uppercase first); unexported `lowerCamelCase`.
- Return `error` values; avoid panics in non-`main` code; keep functions cohesive.

## Testing Guidelines
- Framework: standard `testing`.
- File names: `*_test.go`; functions `TestXxx(t *testing.T)`.
- Keep tests deterministic; prefer table-driven tests; cover new/changed logic.
- Quick check: `go test ./game ./server` and ensure green before PRs.

## Commit & Pull Request Guidelines
- Use Conventional Commits where possible: `feat:`, `fix:`, `refactor:`, `docs:`, `chore:` with optional scope (e.g., `server:`).
- One logical change per commit; present tense, imperative; ~72 char subject.
- PRs must include: clear description, rationale, test plan/steps, linked issues. Add screenshots/GIFs for `static/` UI changes.
- Pre-submit: `go fmt ./...` and `go test ./...` must pass.

## Security & Configuration Tips (Optional)
- No secrets in repo. Configure via env vars/Docker (`docker-compose.yml`).
- Default port is `8080`; override with `-port` flag or compose mapping.

