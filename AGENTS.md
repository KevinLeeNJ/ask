# Repository Guidelines

## Project Structure & Module Organization

`ask` is a Go 1.25 terminal AI question-and-answer CLI. The executable starts at `cmd/ask/main.go` and is wired in `internal/bootstrap`. Application use cases live in `internal/application`, with interfaces in `internal/application/ports`. Keep `internal/domain` independent of concrete implementations. Provider, SQLite, rendering, shell, CLI, and TUI adapters live in their matching `internal/` packages.

Unit tests normally sit beside their packages as `_test.go` files. Cross-layer tests belong in `test/integration`, while `test/architecture` enforces allowed import direction.

## Build, Test, and Development Commands

Run development commands through the temporary-directory wrapper so generated files stay under `.devtmp/`:

```bash
scripts/run-with-dev-temp.sh go test ./...                 # Run all tests
scripts/run-with-dev-temp.sh go vet ./...                  # Run static checks
scripts/run-with-dev-temp.sh go build -o .devtmp/bin/ask ./cmd/ask
scripts/run-with-dev-temp.sh go run ./cmd/ask --help       # Run locally
```

Run a focused test with `scripts/run-with-dev-temp.sh go test ./internal/application/ask`. Do not commit files under `.devtmp/` except `.gitkeep`.

## Coding Style & Naming Conventions

Use `gofmt` output and idiomatic Go naming: short lowercase package names, PascalCase exported identifiers, and mixedCaps for internal names. Keep changes small and place behavior behind existing ports rather than importing concrete adapters into application or domain code. No separate linter configuration is present; `go vet ./...` is the required static check.

## Testing Guidelines

Use table-driven tests where they clarify behavior. Name tests `TestSubject` or `TestSubject_Scenario`, and use the package under test unless an external test package is needed. Add focused tests for behavior changes; integration and architecture tests must also pass.

## Commit & Pull Request Guidelines

The visible history uses `release: v0.1.0`; GoReleaser's changelog rules also recognize prefixes such as `feat:`, `fix:`, `docs:`, `test:`, and `chore:`. Write imperative messages, for example `fix: preserve partial responses`. Pull requests should explain behavior, identify linked issues, list verification commands, and include terminal output when user-visible output changes. Version tags such as `v0.1.1` trigger the release workflow.

## Security & Configuration

Never commit API keys or user conversation data. Credentials belong in environment variables; configuration and SQLite data stay outside the repository in the user config directory.
