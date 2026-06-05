#!/usr/bin/env bash
# Uninstall cs: removes the binary symlink and shell integration
# Usage: bash uninstall.sh
set -euo pipefail

echo "=== cs uninstall ==="

# 1. Remove symlink
BIN="${HOME}/.local/bin/cs"
if [[ -L "$BIN" ]]; then
  rm "$BIN"
  echo "[1/2] removed: ${BIN}"
elif [[ -f "$BIN" ]]; then
  rm "$BIN"
  echo "[1/2] removed (was a copy, not a symlink): ${BIN}"
else
  echo "[1/2] not found, skipping: ${BIN}"
fi

# 2. Remove shell integration from ~/.zshrc
ZSHRC="${HOME}/.zshrc"
if [[ -f "$ZSHRC" ]]; then
  if grep -qF '"${HOME}/.local/bin/cs" init zsh' "$ZSHRC" 2>/dev/null; then
    sed -i '/^# Claude Code provider switcher$/{N;/\.local\/bin\/cs.*init zsh/d}' "$ZSHRC"
    echo "[2/2] removed shell integration from ~/.zshrc"
  else
    echo "[2/2] shell integration not found in ~/.zshrc, skipping"
  fi
else
  echo "[2/2] ~/.zshrc not found, skipping"
fi

# 3. Offer to remove CS_HOME (~/.claude-switch)
CS_HOME="${CS_HOME:-${HOME}/.claude-switch}"
echo ""
if [[ -d "$CS_HOME" ]]; then
  read -r -p "Remove ${CS_HOME}? (contains your provider keys) [y/N] " reply
  if [[ "${reply}" =~ ^[Yy]$ ]]; then
    rm -rf "$CS_HOME"
    echo "removed: ${CS_HOME}"
  else
    echo "kept: ${CS_HOME}"
  fi
fi

echo ""
echo "=== Done ==="
echo "Open a new terminal (or run: source ~/.zshrc)"
