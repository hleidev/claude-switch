# claude-switch

[中文](README.zh.md)

Per-terminal Claude Code provider switcher. New terminals load the default provider automatically; switch to any other provider mid-session with a single command. Closing the terminal resets to the default.

## File structure

```
claude-switch/
├── claude.zsh                   # Main script - source this in ~/.zshrc
├── claude.local.zsh.example     # Default provider template (copy -> claude.local.zsh)
├── claude.local.zsh             # Your default provider loader (gitignored)
├── providers/
│   ├── minimax.zsh.example      # MiniMax config template
│   ├── minimax.zsh              # Your MiniMax config (gitignored)
│   ├── deepseek.zsh.example     # DeepSeek config template
│   └── deepseek.zsh             # Your DeepSeek config (gitignored)
└── install.sh                   # One-time setup script
```

## First-time setup

**1. Create your provider config files**

```bash
cp providers/minimax.zsh.example providers/minimax.zsh
# Edit providers/minimax.zsh and fill in MINIMAX_API_KEY

cp providers/deepseek.zsh.example providers/deepseek.zsh
# Edit providers/deepseek.zsh and fill in DEEPSEEK_API_KEY
```

**2. Run the install script**

```bash
bash install.sh
```

The script will:
- Register each provider's API key in Claude Code's approved list
- Create `claude.local.zsh` (sets the default provider for new terminals)
- Check Claude (claude.ai) OAuth login status and prompt if needed
- Write the source line to `~/.zshrc`

**3. Reload your shell**

```bash
source ~/.zshrc
```

## Usage

| Command | Effect |
|---|---|
| `cs use minimax` | Switch current terminal to MiniMax |
| `cs use deepseek` | Switch current terminal to DeepSeek |
| `cs use claude` | Switch current terminal to Claude (claude.ai) |
| `cs status` | Show current provider and env vars |

- New terminals load the **default provider** set in `claude.local.zsh`
- Different terminal windows can use different providers simultaneously
- Switching only affects **newly started** `claude` instances; running sessions are unaffected

## How it works

Claude Code picks the backend based on env var priority:

- `ANTHROPIC_AUTH_TOKEN` set -> API key mode (uses `ANTHROPIC_BASE_URL`)
- `ANTHROPIC_AUTH_TOKEN` unset -> OAuth mode (reads `~/.claude/.credentials.json`)

`cs use <provider>` sources the provider's config file to set the vars. `cs use claude` unsets them all. Each terminal has its own environment, so windows are fully isolated.

## Adding a provider

Copy an existing example as a starting point:

```bash
# Template (committed, shows up in tab completion)
cp providers/minimax.zsh.example providers/openai.zsh.example
# Edit providers/openai.zsh.example — replace endpoint, models, and key var name

# Your config with the real key (gitignored)
cp providers/openai.zsh.example providers/openai.zsh
# Edit providers/openai.zsh and fill in your API key
```

Then register the key:

```bash
bash install.sh
```

Tab completion reads `providers/*.zsh.example` to list available providers. If you only create the `.zsh` file, the provider works but won't appear in completions.

## New machine setup

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
cp providers/minimax.zsh.example providers/minimax.zsh
# Fill in your API key(s)
bash install.sh
source ~/.zshrc
```
