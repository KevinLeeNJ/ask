# Ask Development Rules

## Scope

- `ask` is a terminal quick-question tool, not a repository-scoped CLI coding agent.
- Keep the ordinary `ask <question>` path free of repository scans, model discovery, agent planning, and unrelated work.

## Development Temporary Files

- `.devtmp/` is the only repository directory allowed for development temporary files.
- Run build, test, coverage, profiling, and other development commands through:

  ```bash
  scripts/run-with-dev-temp.sh <command> [args...]
  ```

- Put intended build output under `.devtmp/`, for example:

  ```bash
  scripts/run-with-dev-temp.sh go build -o .devtmp/bin/ask ./cmd/ask
  ```

- Do not create temporary artifacts in `.cache/`, `tmp/`, `temp/`, `coverage/`, `dist/`, `build/`, `bin/`, or the repository root.
- Do not commit anything under `.devtmp/` except `.gitkeep`.
- `scripts/run-with-dev-temp.sh` redirects `TMPDIR`, `TMP`, and `TEMP` to `.devtmp/` and cleans it on exit, failure, or interruption.

## Verification

- Add focused tests with each behavior change.
- Before committing a phase, run its tests and `scripts/run-with-dev-temp.sh go test ./...` when the Go module exists.
- Verify the worktree and staged diff before each phase commit.
