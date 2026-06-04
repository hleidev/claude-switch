#!/usr/bin/env bash
# One-time setup script. Run once on each new machine.
# Usage: bash install.sh
set -euo pipefail

if ! command -v python3 &>/dev/null; then
  echo "[error] python3 is required. Install it first: brew install python3 / apt install python3"
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Claude Switch Setup ==="

# 1. Check that at least one provider is configured
echo "[1/5] Checking provider configuration..."
PROVIDER_FILES=("${SCRIPT_DIR}/providers/"*.zsh)
CONFIGURED=()
for f in "${PROVIDER_FILES[@]}"; do
  [[ -f "$f" ]] && CONFIGURED+=("$f")
done

if [[ ${#CONFIGURED[@]} -eq 0 ]]; then
  echo ""
  echo "      No providers configured. Create at least one from a template:"
  echo ""
  for example in "${SCRIPT_DIR}/providers/"*.zsh.example; do
    name="$(basename "${example%.zsh.example}")"
    echo "      cp providers/${name}.zsh.example providers/${name}.zsh"
    echo "      # edit providers/${name}.zsh and fill in your API key"
  done
  echo ""
  echo "      Then re-run this script."
  exit 1
fi

NAMES=()
for f in "${CONFIGURED[@]}"; do NAMES+=("$(basename "${f%.zsh}")"); done
echo "      Found: ${NAMES[*]}"

# 2. Register all provider API keys in customApiKeyResponses
echo "[2/5] Registering API keys in customApiKeyResponses..."

for PROVIDER_ZSH in "${CONFIGURED[@]}"; do
  PROVIDER_NAME="$(basename "${PROVIDER_ZSH%.zsh}")"
  KEY="$(zsh -c "source '${PROVIDER_ZSH}' 2>/dev/null; printf '%s' \"\${ANTHROPIC_AUTH_TOKEN:-}\"")"

  if [[ -z "${KEY}" ]]; then
    echo "      [skip] ${PROVIDER_NAME}: ANTHROPIC_AUTH_TOKEN not set"
    continue
  fi

  if [[ "${KEY}" == *"your-"*"-api-key-here"* ]]; then
    echo "      [skip] ${PROVIDER_NAME}: API key not filled in"
    continue
  fi

  KEY_SUFFIX="${KEY: -20}"
  _registered=0

  for CLAUDE_JSON in "$HOME/.claude.json" "$HOME/.claude/claude.json"; do
    mkdir -p "$(dirname "$CLAUDE_JSON")"
    if [[ -f "$CLAUDE_JSON" ]]; then
      if grep -q "$KEY_SUFFIX" "$CLAUDE_JSON" 2>/dev/null; then
        continue
      fi
      python3 - "$CLAUDE_JSON" "$KEY_SUFFIX" <<'PYEOF'
import sys, json
path, suffix = sys.argv[1], sys.argv[2]
with open(path) as f:
    data = json.load(f)
data.setdefault("customApiKeyResponses", {}).setdefault("approved", [])
if suffix not in data["customApiKeyResponses"]["approved"]:
    data["customApiKeyResponses"]["approved"].append(suffix)
with open(path, "w") as f:
    json.dump(data, f, indent=2)
PYEOF
      _registered=$((_registered + 1))
    else
      echo "{\"customApiKeyResponses\":{\"approved\":[\"${KEY_SUFFIX}\"],\"rejected\":[]}}" \
        | python3 -m json.tool > "$CLAUDE_JSON"
      _registered=$((_registered + 1))
    fi
  done

  if [[ $_registered -gt 0 ]]; then
    echo "      ${PROVIDER_NAME}: registered"
  else
    echo "      ${PROVIDER_NAME}: already registered"
  fi
done

# 3. Create claude.local.zsh (default provider loader) if missing
echo "[3/5] Checking default provider config (claude.local.zsh)..."
if [[ ! -f "${SCRIPT_DIR}/claude.local.zsh" ]]; then
  cp "${SCRIPT_DIR}/claude.local.zsh.example" "${SCRIPT_DIR}/claude.local.zsh"
  echo "      Created claude.local.zsh (defaults to minimax)"
  echo "      Edit it to change the default provider."
else
  echo "      claude.local.zsh already exists, skipping"
fi

# 4. Claude Pro (OAuth) login
echo "[4/5] Checking Claude (claude.ai) login..."
if [[ -f "$HOME/.claude/.credentials.json" ]]; then
  echo "      ~/.claude/.credentials.json found, skipping"
else
  echo ""
  echo "      Starting a Claude Code session to complete OAuth login:"
  echo "      1. Type /login and press Enter"
  echo "      2. Complete authorization in the browser"
  echo "      3. Type /exit when done"
  echo ""
  read -r -p "      Press Enter when ready..."
  claude
fi

# 5. Write source line to ~/.zshrc
echo "[5/5] Checking ~/.zshrc..."
ZSHRC="$HOME/.zshrc"
SOURCE_LINE="[[ -f \"${SCRIPT_DIR}/claude.zsh\" ]] && source \"${SCRIPT_DIR}/claude.zsh\""
if grep -qF "claude-switch/claude.zsh" "$ZSHRC" 2>/dev/null; then
  echo "      Source line already present, skipping"
else
  echo "" >> "$ZSHRC"
  echo "# Claude provider switcher" >> "$ZSHRC"
  echo "${SOURCE_LINE}" >> "$ZSHRC"
  echo "      Written to ~/.zshrc"
fi

echo ""
echo "=== Setup complete ==="
echo "Run 'source ~/.zshrc' or open a new terminal to apply."
echo "Commands: cs use minimax | cs use deepseek | cs use claude | cs status"
