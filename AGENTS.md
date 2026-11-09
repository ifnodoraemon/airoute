# Repository Guidelines

## Project Structure & Module Organization

Airoute is a Go AI gateway with an embedded React console.

- `cmd/airoute/`: server entry point and CLI commands.
- `internal/`: API handlers, control plane, routing, providers, storage, and telemetry.
- `web/src/`: React components, hooks, API clients, and colocated tests; `web/public/`: static assets.
- `tests/`: gateway integration tests; `tests/e2e/`: live-cluster tests.
- `configs/`, `deploy/`, `helm/`, and `scripts/`: configuration, deployment, and automation; `docs/`: architecture and engineering guidance.

## Build, Test, and Development Commands

Use the Go version in `go.mod` and Node.js 22, matching CI.

- `make build`: build the console and embedded gateway at `bin/airoute`.
- `make build-web`: install frontend dependencies and build `web/dist/`, required before direct Go compilation.
- `docker compose up -d --build`: start the configured local cluster.
- `make test`: run Go tests with the race detector, preparing frontend assets automatically.
- `make test-web`: run frontend tests after installing dependencies.
- `cd web && npm run lint`: run ESLint with zero warnings allowed.
- `make test-integration`: test PostgreSQL/Redis paths; requires `AIROUTE_TEST_DATABASE_URL` and `AIROUTE_TEST_REDIS_URL`.

## Coding Style & Naming Conventions

Use `gofmt` and tabs for Go; follow two-space indentation, single quotes, and semicolons in JavaScript/JSX. Use descriptive `snake_case.go` filenames, PascalCase React components, and `useCamelCase` hooks.

Follow [engineering guidelines](docs/ENGINEERING_GUIDELINES.md): keep business files within 300 lines and functions within 60 lines, respecting documented exceptions. Separate streaming and unary logic. Keep protocol errors in English, structured log messages in lowercase English, and control-plane/CLI messages in technical Chinese.

## Testing Guidelines

Use Go `testing`/testify with `*_test.go` and `TestXxx`; frontend tests use Vitest, React Testing Library, and jsdom in `*.test.js` or `*.test.jsx`. Add regressions for changed behavior. CI records Go coverage without a numeric threshold. Verify affected PostgreSQL/Redis paths against real services. Run `make test-e2e-go` against a running cluster. See [.github/workflows/ci.yml](.github/workflows/ci.yml) for complete checks.

## Commit & Pull Request Guidelines

Follow history's Conventional Commit style: `fix(console): ...`, `feat(thinking): ...`, or `refactor(thinking): ...`. PRs should describe behavior changes, link relevant issues, list validation, and include screenshots for UI changes. Update API, frontend, tests, and documentation together.

## Security & Configuration

Create `.env` from `.env.example` and supply credentials before starting Compose. Keep secrets, databases, binaries, and generated `web/dist/` out of commits. Report vulnerabilities privately through the process in `SECURITY.md`.
