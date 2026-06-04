#!/usr/bin/env bash
# One-time setup: installs cs to ~/.local/bin and adds shell integration to ~/.zshrc
# Usage: bash install.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== cs install ==="

# 1. Check python3 (required by cs for API key registration)
if ! command -v python3 &>/dev/null; then
  echo "[error] python3 is required."
  if [[ "$(uname -s)" == "Darwin" ]]; then
    echo "  macOS: xcode-select --install  (or: brew install python3)"
  else
    echo "  Linux: apt install python3 / dnf install python3 / pacman -S python"
  fi
  exit 1
fi

# 2. Install cs to ~/.local/bin
INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "$INSTALL_DIR"
cp "${SCRIPT_DIR}/cs" "${INSTALL_DIR}/cs"
chmod +x "${INSTALL_DIR}/cs"
echo "[1/2] installed: ${INSTALL_DIR}/cs"

# 3. Configure ~/.zshrc (PATH export + shell integration)
ZSHRC="${HOME}/.zshrc"
PATH_LINE='export PATH="${HOME}/.local/bin:${PATH}"'
ACTIVATE_LINE='command -v cs &>/dev/null && eval "$(cs init zsh)"'

has_activate=0; grep -qF "cs init zsh"                              "$ZSHRC" 2>/dev/null && has_activate=1
has_path=0;    grep -qE 'export PATH=.*\.local/bin'                 "$ZSHRC" 2>/dev/null && has_path=1

if [[ $has_activate -eq 1 && $has_path -eq 1 ]]; then
  echo "[2/2] ~/.zshrc already configured"
elif [[ $has_activate -eq 1 ]]; then
  # PATH missing, activation present: insert PATH before the activation block
  awk -v line="$PATH_LINE" '
    /^# Claude Code provider switcher$/ && !done {
      print "# Add ~/.local/bin to PATH (Claude Code provider switcher)"
      print line
      print ""
      done=1
    }
    { print }
  ' "$ZSHRC" > "${ZSHRC}.cs-tmp" && mv "${ZSHRC}.cs-tmp" "$ZSHRC"
  echo "[2/2] added PATH export to ~/.zshrc"
elif [[ $has_path -eq 1 ]]; then
  printf '\n# Claude Code provider switcher\n%s\n' "$ACTIVATE_LINE" >> "$ZSHRC"
  echo "[2/2] added shell integration to ~/.zshrc"
else
  printf '\n# Add ~/.local/bin to PATH (Claude Code provider switcher)\n%s\n\n# Claude Code provider switcher\n%s\n' "$PATH_LINE" "$ACTIVATE_LINE" >> "$ZSHRC"
  echo "[2/2] written to ~/.zshrc"
fi

echo ""
echo "=== Done ==="
echo "Run:  source ~/.zshrc"
echo "Then: cs add minimax    (or: cs add deepseek)"
