# Claude Code provider switcher
# Add to the end of ~/.zshrc:
#   [[ -f ~/Personal/claude-switch/claude.zsh ]] && source ~/Personal/claude-switch/claude.zsh

_CLAUDE_SWITCH_DIR="$(cd "$(dirname "${(%):-%x}")" && pwd)"

# Load default provider (gitignored)
[[ -f "${_CLAUDE_SWITCH_DIR}/claude.local.zsh" ]] && source "${_CLAUDE_SWITCH_DIR}/claude.local.zsh"

function cs() {
  local cmd="$1"
  local target="$2"

  case "$cmd" in
    use)
      case "$target" in
        claude|pro)
          unset CLAUDE_CONFIG_DIR
          unset ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN
          unset ANTHROPIC_MODEL ANTHROPIC_SMALL_FAST_MODEL
          unset ANTHROPIC_DEFAULT_SONNET_MODEL
          unset ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL
          unset API_TIMEOUT_MS CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
          unset CLAUDE_CODE_EFFORT_LEVEL
          unset MINIMAX_API_KEY DEEPSEEK_API_KEY
          unset CLAUDE_SWITCH_PROVIDER
          echo "[claude-switch] switched to Claude (claude.ai)"
          ;;
        *)
          local _pfile="${_CLAUDE_SWITCH_DIR}/providers/${target}.zsh"
          if [[ -f "${_pfile}" ]]; then
            source "${_pfile}"
            echo "[claude-switch] switched to ${target}"
          elif [[ -f "${_pfile}.example" ]]; then
            echo "[claude-switch] provider '${target}' not configured"
            echo "  cp providers/${target}.zsh.example providers/${target}.zsh"
            echo "  then fill in your API key"
          else
            local -a _available=(claude)
            for _p in "${_CLAUDE_SWITCH_DIR}/providers/"*.zsh(N); do
              _available+=("${_p:t:r}")
            done
            echo "unknown provider: ${target}"
            echo "available: ${(j:, :)_available}"
          fi
          ;;
      esac
      ;;
    status)
      local _provider
      if [[ -n "${ANTHROPIC_AUTH_TOKEN}" ]]; then
        _provider="${CLAUDE_SWITCH_PROVIDER:-unknown (${ANTHROPIC_BASE_URL:-no URL})}"
      else
        _provider="claude"
      fi
      echo "Active provider  : ${_provider}"
      echo "Base URL         : ${ANTHROPIC_BASE_URL:-(unset)}"
      echo "Auth token       : $([[ -n "${ANTHROPIC_AUTH_TOKEN}" ]] && echo 'set (hidden)' || echo 'unset')"
      echo "Model            : ${ANTHROPIC_MODEL:-(unset)}"
      echo "Small/fast model : ${ANTHROPIC_SMALL_FAST_MODEL:-(unset)}"
      echo "Default sonnet   : ${ANTHROPIC_DEFAULT_SONNET_MODEL:-(unset)}"
      echo "Default opus     : ${ANTHROPIC_DEFAULT_OPUS_MODEL:-(unset)}"
      echo "Default haiku    : ${ANTHROPIC_DEFAULT_HAIKU_MODEL:-(unset)}"
      echo "Config dir       : ${CLAUDE_CONFIG_DIR:-~/.claude (default)}"
      echo "Disable traffic  : ${CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC:-(unset)}"
      echo "Effort level     : ${CLAUDE_CODE_EFFORT_LEVEL:-(unset)}"
      ;;
    *)
      echo "usage: cs use <provider> | cs status"
      ;;
  esac
}

function _cs() {
  case $CURRENT in
    2)
      local -a cmds
      cmds=('use:switch provider' 'status:show current provider')
      _describe 'command' cmds
      ;;
    3)
      case $words[2] in
        use)
          local -a providers
          providers=('claude:Claude.ai (OAuth)')
          for _f in "${_CLAUDE_SWITCH_DIR}/providers/"*.zsh.example(N); do
            providers+=("${_f:t:r:r}")
          done
          _describe 'provider' providers
          ;;
      esac
      ;;
  esac
}
(( $+functions[compdef] )) && compdef _cs cs
