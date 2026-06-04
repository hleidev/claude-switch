# cs — Claude Code provider switcher

[中文](README.zh.md)

Per-terminal Claude Code provider switcher. Switch between Claude.ai (OAuth) and third-party API providers (MiniMax, DeepSeek, …) with a single command. Each terminal window is independent.

## How it works

Claude Code picks the backend based on environment variables:

- `ANTHROPIC_AUTH_TOKEN` set → API key mode (uses `ANTHROPIC_BASE_URL`)
- `ANTHROPIC_AUTH_TOKEN` unset → OAuth mode (reads `~/.claude/.credentials.json`)

`cs use <provider>` sources the provider config into the current shell. `cs use claude` unsets everything to fall back to OAuth. Each terminal has its own environment, so windows are fully isolated.

## Prerequisites

- **zsh** — macOS Catalina+ default; verify with `echo $SHELL`
- **python3** — for API key registration
  - macOS: `xcode-select --install` (includes python3) or `brew install python3`
  - Linux: usually pre-installed; otherwise `apt install python3` / `dnf install python3`
- **claude CLI** — Claude Code must be installed and `claude` in your PATH

## Install

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
bash install.sh
source ~/.zshrc
```

`install.sh` copies `cs` to `~/.local/bin/cs` and adds the shell integration line to `~/.zshrc`.

## First-time setup

**Add a provider:**

```bash
cs add minimax    # opens $EDITOR to fill in your API key
```

**Set a default** (loads automatically in every new terminal):

```bash
cs default minimax
```

**Log in to Claude.ai** (for OAuth mode):

```bash
cs add claude
```

## Usage

| Command | Effect |
|---|---|
| `cs use minimax` | Switch current terminal to MiniMax |
| `cs use deepseek` | Switch current terminal to DeepSeek |
| `cs use claude` | Switch current terminal to Claude.ai (OAuth) |
| `cs default minimax` | Set MiniMax as default for new terminals |
| `cs default` | Show current global default |
| `cs list` | List installed providers |
| `cs status` | Show current provider and env vars |
| `cs add minimax` | Add MiniMax provider (opens editor) |
| `cs edit minimax` | Edit MiniMax config (model name, key, etc.) |
| `cs remove minimax` | Remove MiniMax provider |

- New terminals load the **global default** set by `cs default`
- Different terminal windows can use different providers simultaneously
- Switching only affects **newly started** `claude` instances

## Provider config location

All provider configs are stored in `~/.claude-switch/providers/`. The project repo contains no secrets.

## Adding a provider

Built-in templates: `minimax`, `deepseek`, `claude`

```bash
cs add minimax    # creates ~/.claude-switch/providers/minimax.zsh and opens $EDITOR
```

To update a provider config (model name changed, key rotated, etc.):

```bash
cs edit minimax
```

## New machine setup

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
bash install.sh
source ~/.zshrc
cs add minimax    # re-enter your API key
cs default minimax
```

## Testing

```bash
brew install bats-core
bats test/
```

Tests use an isolated `CS_HOME` under `/tmp/cs-test-*` and never touch `~/.claude-switch/`. Temporary directories are removed automatically after each test.
