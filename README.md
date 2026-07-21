<div align="center">

# claude-switch (`cs`)

**Switch Claude Code's backend per terminal — one command, no global config.**

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)

[Install](#installation) · [Quick start](#quick-start) · [Commands](#commands) · [中文](README.zh.md)

</div>

---

Switch Claude Code between Claude.ai (OAuth) and third-party API providers per
terminal, with one command. Each terminal window is independent, so you can run
one provider in a shell and another in the next. Native `settings.json` can't do
this; it's global.

Everything is `cs`. `cs setup` adds a shell function that shadows the binary of
the same name — `cs use` needs to inject env into your shell, which a child
process cannot do; every other subcommand forwards straight to the binary.

## How it works

Claude Code picks its backend from environment variables:

- `ANTHROPIC_AUTH_TOKEN` set → API-key mode (uses `ANTHROPIC_BASE_URL`)
- `ANTHROPIC_AUTH_TOKEN` unset → OAuth mode (`~/.claude/.credentials.json`)

`cs use <provider>` injects a provider's env into the current shell; `cs use
claude` clears it and falls back to OAuth. Switching only affects **newly
started** `claude` instances in that terminal.

## Features

- Switch one terminal without touching other windows or the global `settings.json`
- Built-in presets for minimax, deepseek, glm, and anthropic: add a key and you're set
- `cs use claude` falls back to your Claude.ai OAuth login
- Keys live in a `0600` config file and never show up in `cs list` or `cs status`
- The installer sets up shell integration for zsh and bash

## Installation

### Homebrew (macOS)

```bash
brew tap hleidev/claude-switch
brew install --cask claude-switch
cs setup              # wires the `cs` shell function into your rc file
exec $SHELL
```

> Linux: Homebrew casks are macOS-only. Download the binary for your arch from the
> [latest release](https://github.com/hleidev/claude-switch/releases/latest), or
> build [from source](#from-source).

### From source

Requires Go 1.26+ and a POSIX shell (zsh or bash). See
[Development](#development) for the full build/test workflow.

```bash
git clone https://github.com/hleidev/claude-switch.git
cd claude-switch
make install          # builds, installs to ~/.local/bin, wires up your rc file
exec $SHELL           # or open a new terminal
```

Override the install prefix like any autotools-style project: `make install PREFIX=/usr/local`.

## Quick start

```bash
cs add                # pick a provider, paste your key
cs use glm            # this terminal now routes to GLM
cs list               # ✓ default, ● this terminal
claude                # ...uses the selected provider
```

`cs use claude` switches back to OAuth. The choice sticks for new `claude`
processes started in this terminal; other terminals are unaffected.

## Commands

| Command | Effect |
|---|---|
| `cs add [provider]` | Add a provider (interactive picker + hidden key entry; `--key-stdin` to script, `--base-url` for custom) |
| `cs use <provider>` | Switch this terminal to a provider |
| `cs use claude` | Reset this terminal to Claude.ai (OAuth) |
| `cs default [provider]` | Show / set the provider new terminals load |
| `cs list` | List providers (✓ default, ● this terminal) |
| `cs status` | Current terminal's provider and config summary |
| `cs edit [provider]` | Open the whole config in `$EDITOR` |
| `cs remove <provider>` | Remove a provider |
| `cs doctor` | Diagnose the setup |
| `cs migrate` | Import from the legacy `~/.claude-switch` layout |
| `cs version` | Print the version |

Built-in presets: `minimax`, `deepseek`, `glm`, `anthropic`. Anything else is a
`custom…` provider that needs its own base URL (`--base-url`).

## Configuration

A single file at `${XDG_CONFIG_HOME:-~/.config}/claude-switch/config.toml`
(`0600`). A provider is just a flat table of environment variables, keyed by
their real names. Model names, timeouts, and other defaults for built-in presets
are maintained by the project (see `internal/presets/data/presets.toml`), so a
preset provider's config usually only needs the secret:

```toml
version = 2
default_provider = "glm"

[providers.glm]
ANTHROPIC_AUTH_TOKEN = "sk-..."
# To override a preset variable, write that line (it wins over the preset):
# ANTHROPIC_MODEL = "glm-4.7"
```

`cs use` exports the merge of `defaults → preset → your overrides`. A custom
(non-preset) provider has no template, so it must supply its own
`ANTHROPIC_BASE_URL`. Edit the file with `cs edit`. Keys are never printed by
`cs list` / `cs status`.

## Updating

Presets (built-in model names, base URLs, etc.) are embedded into the binary at
build time, so updating means reinstalling the binary, not editing a config
file.

### Homebrew

```bash
brew update && brew upgrade --cask claude-switch
cs version            # confirm the new version
```

If you ever installed a pre-release build whose binary was named
`claude-switch`, terminals still holding that old shell function will report
`command not found: claude-switch`. Fix it once with:

```bash
command cs setup      # bypasses the stale function; rewrites your rc line
exec $SHELL
```

### From source

```bash
cd claude-switch
git pull
make install          # rebuilds, overwrites ~/.local/bin/cs
cs version            # confirm the new version
```

Providers you've already added with `cs add` don't follow preset changes — their
model names are written into your config. To refresh one to the latest preset
defaults, run `cs add <provider> --force` (re-enter the key) or edit it with
`cs edit`.

## Migrating from the old bash version

If you used the previous unversioned bash `cs` (data in `~/.claude-switch/`):

```bash
cs migrate
```

It imports your providers and default, re-registers your keys with Claude Code,
and offers to remove the old shell integration. Your old `~/.claude-switch/` is
left untouched until you delete it yourself.

## Uninstall

### Homebrew

```bash
cs uninstall          # removes shell integration; asks about config
brew uninstall --cask claude-switch
```

Order matters: `cs uninstall` needs the binary, so run it first. Removing the
cask first strands the integration line in your rc file.

### From source

```bash
make uninstall        # removes shell integration (asks about config) + the binary
```

## Development

For contributors — if you just want to use `cs`, the
[Installation](#installation) section is all you need.

### Build

```bash
make build            # -> bin/cs
```

The Makefile injects the version via `-ldflags "-X main.version=$(VERSION)"`,
where `VERSION` is `git describe --tags --always --dirty` (so a dev build prints
the current commit SHA; a tagged build prints the tag).

### Test

```bash
make test             # go test ./...
```

### Format & vet

```bash
make fmt              # gofmt -w .
make vet              # go vet ./...
```

### Project layout

| Path | Purpose |
|---|---|
| `main.go` | Entry point; reads `main.version` and hands off to `cmd` |
| `cmd/` | Cobra subcommand implementations (`add`, `use`, `setup`, …) |
| `internal/claudejson` | Reads/writes `~/.claude/.credentials.json` for OAuth re-registration |
| `internal/config` | Parses `config.toml`, providers, defaults |
| `internal/migrate` | One-shot importer for the legacy `~/.claude-switch/` layout |
| `internal/presets` | Built-in provider templates (model names, base URLs) |
| `internal/shellenv` | Env-var merge + `init zsh` / `init bash` snippets |
| `.goreleaser.yaml` | GoReleaser config; a tagged push builds binaries and updates the [Homebrew tap](https://github.com/hleidev/homebrew-claude-switch) |
| `Makefile` | `build` / `test` / `install` / `uninstall` / `fmt` / `vet` / `clean` |

## License

MIT — see [LICENSE](LICENSE).
