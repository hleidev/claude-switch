# claude-switch (`cs`)

[中文](README.zh.md)

Per-terminal Claude Code provider switcher. Switch between Claude.ai (OAuth) and
third-party API providers (MiniMax, DeepSeek, …) with one command — **each
terminal window is independent**, which native `settings.json` cannot do.

The PATH binary is `claude-switch`; you drive it through the short `cs` shell
function that the installer adds (it exists because env injection must happen in
your shell, not a child process).

## How it works

Claude Code picks its backend from environment variables:

- `ANTHROPIC_AUTH_TOKEN` set → API-key mode (uses `ANTHROPIC_BASE_URL`)
- `ANTHROPIC_AUTH_TOKEN` unset → OAuth mode (`~/.claude/.credentials.json`)

`cs use <provider>` injects a provider's env into the current shell; `cs use
claude` clears it and falls back to OAuth. Switching only affects **newly
started** `claude` instances in that terminal.

## Requirements

- **Go** (to build) — `brew install go`
- **zsh or bash** — interactive shell integration

## Install

```bash
git clone https://github.com/hleidev/claude-switch.git
cd claude-switch
make install          # builds, installs to ~/.local/bin, wires up your rc file
exec $SHELL           # or open a new terminal
```

Override the location like any autotools-style project: `make install PREFIX=/usr/local`.

## Migrating from the old bash version

If you used the previous unversioned bash `cs` (data in `~/.claude-switch/`):

```bash
cs migrate
```

It imports your providers and default, re-registers your keys with Claude Code,
and offers to remove the old shell integration. Your old `~/.claude-switch/` is
left untouched until you delete it yourself.

## Usage

| Command | Effect |
|---|---|
| `cs add [provider]` | Add a provider (interactive picker + hidden key entry; `--key-stdin` to script) |
| `cs use <provider>` | Switch this terminal to a provider |
| `cs use claude` | Reset this terminal to Claude.ai (OAuth) |
| `cs default [provider]` | Show / set the provider new terminals load |
| `cs set <p> <field> [value]` | Edit one field (`cs set <p> key` prompts hidden for the API key) |
| `cs unset <p> <field>` | Clear one field |
| `cs list` | List providers (✓ default, ● this terminal) |
| `cs status` | Current terminal's provider and config summary |
| `cs edit [provider]` | Open the whole config in `$EDITOR` |
| `cs remove <provider>` | Remove a provider |
| `cs doctor` | Diagnose the setup |
| `cs migrate` | Import from the legacy `~/.claude-switch` layout |
| `cs version` | Print the version |

Built-in presets: `minimax`, `deepseek`, `anthropic`. Anything else is a
`custom…` provider you supply a base URL for.

## Configuration

A single file at `${XDG_CONFIG_HOME:-~/.config}/claude-switch/config.toml`
(`0600`), secrets inline. Edit it directly with `cs edit`, or field-by-field with
`cs set`. Keys are never printed by `cs list` / `cs status`.

## Uninstall

```bash
make uninstall        # removes shell integration (asks about config) + the binary
```

## Development

```bash
make build            # -> bin/claude-switch
make test             # go test ./...
make fmt vet
```
