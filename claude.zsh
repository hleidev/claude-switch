# Claude Code provider 切换脚本
# 在 ~/.zshrc 末尾加一行：
#   [[ -f ~/Personal/claude-switch/claude.zsh ]] && source ~/Personal/claude-switch/claude.zsh

_CLAUDE_SWITCH_DIR="$(cd "$(dirname "${(%):-%x}")" && pwd)"

# 加载本地 token（gitignored）
[[ -f "${_CLAUDE_SWITCH_DIR}/claude.local.zsh" ]] && source "${_CLAUDE_SWITCH_DIR}/claude.local.zsh"

function cs() {
  local cmd="$1"
  local target="$2"

  case "$cmd" in
    use)
      case "$target" in
        claude|pro)
          unset CLAUDE_CONFIG_DIR
          unset ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN MINIMAX_API_KEY
          unset ANTHROPIC_MODEL ANTHROPIC_SMALL_FAST_MODEL
          unset ANTHROPIC_DEFAULT_SONNET_MODEL
          unset ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL
          unset API_TIMEOUT_MS CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
          echo "[claude-switch] switched to Claude (claude.ai)"
          ;;
        minimax)
          source "${_CLAUDE_SWITCH_DIR}/claude.local.zsh"
          echo "[claude-switch] switched to MiniMax"
          ;;
        *)
          echo "unknown provider: $target"
          echo "usage: cs use <claude|minimax>"
          ;;
      esac
      ;;
    status)
      local _provider
      if [[ -n "${ANTHROPIC_AUTH_TOKEN}" ]]; then
        if [[ "${ANTHROPIC_BASE_URL}" == *minimax* ]]; then
          _provider="MiniMax"
        else
          _provider="API key (${ANTHROPIC_BASE_URL:-unknown endpoint})"
        fi
      else
        _provider="Claude (claude.ai)"
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
      ;;
    *)
      echo "usage: cs use <claude|minimax> | cs status"
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
          providers=('claude:Claude.ai (OAuth)' 'minimax:MiniMax API')
          _describe 'provider' providers
          ;;
      esac
      ;;
  esac
}
(( $+functions[compdef] )) && compdef _cs cs
