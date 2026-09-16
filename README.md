# ask

[English](README.md) | [简体中文](README.zh-CN.md)

`ask` is a fast terminal AI question-and-answer tool for quick questions and follow-ups. It does not inspect the current repository or start agent workflows.

## Features

- Ask in one command and automatically preserve conversation context.
- Supports OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages-compatible APIs.
- Streams Markdown, presents reasoning separately, and collapses thinking when the answer begins.
- Stores conversations in local SQLite with selection, renaming, deletion, and retention policies.
- `ask config` provides menu-based setup and hidden API key input.
- Conversations inherit the most recently used provider and model; command flags override them for one request.
- Supports plain-text, piped, JSON, and terminal capability-aware output modes.

## Installation

The install script detects macOS, Linux, `amd64`, and `arm64`, downloads the matching binary from the latest release, and verifies its SHA-256 checksum.

macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh | sh
```

The default install location is `$HOME/.local/bin/ask`. If that directory is not in `PATH`, the script prints the configuration you need to add. You can also choose another directory:

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh |
  sh -s -- --install-dir "$HOME/bin"
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/KevinLeeNJ/ask/main/install.sh |
  ASK_VERSION=v0.1.0 sh
```

On Windows, run the script in Git Bash or WSL, or download the appropriate `.zip` or `.tar.gz` directly from [Releases](https://github.com/KevinLeeNJ/ask/releases).

Verify the installation:

```bash
ask --version
```

## Quick Start

Run the setup wizard on first use:

```bash
ask config
```

The wizard performs these steps:

1. Select OpenAI Chat Completions, OpenAI Responses, or Anthropic Messages.
2. Configure the provider ID, base URL, and request timeout.
3. Enter the API key with hidden input.
4. Fetch and select an available model, or enter a model ID manually.
5. Select the default model.
6. Preview and confirm the shell profile changes.
7. Save the configuration.

The API key is not written to `config.toml` or SQLite. After it is added to your shell profile, run the displayed `source` command or reopen the terminal.

The environment variable name is generated from the provider ID: convert it to uppercase, replace `-` with `_`, and append `_API_KEY`. For example:

```text
opencode       -> OPENCODE_API_KEY
openai-main    -> OPENAI_MAIN_API_KEY
```

After configuration, ask directly:

```bash
ask "Explain Go escape analysis"
ask "Compare PostgreSQL and SQLite"
```

## Common Commands

```bash
# Ask a question; multiple positional arguments are joined into one prompt
ask "What model are you?" "What is your architecture?"

# Start a new conversation
ask --new "Compare PostgreSQL and SQLite"

# Continue a specific conversation by full ID or unique prefix
ask --conversation 0192a "Continue the previous analysis"

# Treat a reserved word as a normal question
ask -- config

# Override the provider and model for one request
ask --provider opencode --model deepseek-v4.1-flash "Analyze this race condition"

# Enable or disable thinking for one request
ask --thinking "Analyze this deadlock"
ask --no-thinking "Translate this sentence into English"

# Manage conversations
ask conversations
ask conversations list

# Machine-readable output
ask --json "Summarize this log"
```

View all flags:

```bash
ask --help
```

## Conversation and Model Routing

An existing conversation reuses the provider and model most recently used in that conversation. `--provider` and `--model` override only the current request and do not change the saved configuration.

The default model is determined by `active_provider` in `config.toml` and the corresponding provider's `default_model`.

## Thinking Mode

`reasoning.mode` supports:

- `auto`: Uses local multilingual heuristics to decide whether thinking is needed without making an extra probe request. When enabled, it uses the current model's lowest effective level.
- `on`: Forces thinking on using the lowest effective level.
- `off`: Disables thinking.

Override it for one request:

```bash
ask --thinking "Analyze this memory leak"
ask --no-thinking "Convert JSON to YAML"
```

When a model is not covered by the built-in capability table, explicitly enabling thinking attempts to fetch public reasoning metadata from models.dev and cache the lowest effective level.

## Output Modes

- `plain`: Raw Markdown without ANSI, suitable for pipes, redirects, and terminals that do not support control sequences.
- `inline`: Appends output without clearing the screen or moving the cursor backward.
- `full`: Uses partial redraws and semantic Markdown rendering in a safe TTY.
- `auto`: Selects a mode based on stdout and terminal capabilities.

stdout carries only the final answer or JSON; progress, reasoning, errors, and usage are written to stderr.

## Configuration

Default configuration paths:

```text
macOS / Linux: $HOME/.config/ask/config.toml
Windows:       %AppData%\ask\config.toml
```

The SQLite conversation database is stored in the same directory:

```text
ask.db
```

Use `ASK_CONFIG_FILE` to select another configuration file:

```bash
ASK_CONFIG_FILE="$HOME/.config/ask/work.toml" ask config
ASK_CONFIG_FILE="$HOME/.config/ask/work.toml" ask "Hello"
```

## Uninstall

If you used the default install directory:

```bash
rm "$HOME/.local/bin/ask"
```

To remove configuration, credentials, and conversation history as well:

```bash
rm -rf "$HOME/.config/ask"
```

If the setup wizard added an API key to your shell profile, manually remove the corresponding managed block.

## Development

Go 1.25 or newer is required.

```bash
go test ./...
go vet ./...
```

Releases are automated with GoReleaser and GitHub Actions. Maintainers tag and push the target commit:

```bash
git tag v0.1.1
git push origin v0.1.1
```

The workflow builds `amd64` and `arm64` binaries for macOS, Linux, and Windows, creates checksum files, and publishes a GitHub Release.

## License

[MIT](LICENSE)
