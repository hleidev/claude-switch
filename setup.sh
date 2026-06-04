#!/usr/bin/env bash
# 一次性初始化脚本，在每台新机器上运行一次
# 用法：bash setup.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Claude Switch 初始化 ==="

# 1. 检查 claude.local.zsh 是否存在
if [[ ! -f "${SCRIPT_DIR}/claude.local.zsh" ]]; then
  echo "[1/4] 创建 claude.local.zsh（从模板复制）..."
  cp "${SCRIPT_DIR}/claude.local.zsh.example" "${SCRIPT_DIR}/claude.local.zsh"
  echo "      请编辑 ${SCRIPT_DIR}/claude.local.zsh，填入真实 token，然后重新运行本脚本"
  exit 1
fi

# 加载 local 配置读取 MINIMAX_API_KEY
source "${SCRIPT_DIR}/claude.local.zsh"

if [[ "${MINIMAX_API_KEY}" == "your-minimax-api-key-here" || -z "${MINIMAX_API_KEY}" ]]; then
  echo "[错误] claude.local.zsh 中的 MINIMAX_API_KEY 尚未填写，请先填入真实 token"
  exit 1
fi

# 2. 写入 MiniMax key 到 customApiKeyResponses（两个路径都写，兼容不同版本）
KEY_SUFFIX="${MINIMAX_API_KEY: -20}"
echo "[2/4] 注册 MiniMax API key 到 customApiKeyResponses..."

for CLAUDE_JSON in "$HOME/.claude.json" "$HOME/.claude/claude.json"; do
  mkdir -p "$(dirname "$CLAUDE_JSON")"
  if [[ -f "$CLAUDE_JSON" ]]; then
    # 检查是否已存在
    if grep -q "$KEY_SUFFIX" "$CLAUDE_JSON" 2>/dev/null; then
      echo "      ${CLAUDE_JSON} 已包含该 key，跳过"
      continue
    fi
    # 追加到 approved 数组（用 python 做 JSON 处理）
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
    echo "      已更新 ${CLAUDE_JSON}"
  else
    # 文件不存在，直接创建
    echo "{\"customApiKeyResponses\":{\"approved\":[\"${KEY_SUFFIX}\"],\"rejected\":[]}}" \
      | python3 -m json.tool > "$CLAUDE_JSON"
    echo "      已创建 ${CLAUDE_JSON}"
  fi
done

# 3. 登录 Claude Pro（OAuth）
echo "[3/4] 检查 Claude (claude.ai) 登录状态..."
if [[ -f "$HOME/.claude/.credentials.json" ]]; then
  echo "      ~/.claude/.credentials.json 已存在，跳过"
else
  echo ""
  echo "      即将进入 Claude Code 会话，请按以下步骤完成登录："
  echo "      1. 会话启动后，输入 /login 并回车"
  echo "      2. 在浏览器完成 Claude Pro 授权"
  echo "      3. 授权完成后，输入 /exit 退出会话"
  echo ""
  read -r -p "      准备好后按回车继续..."
  claude
fi

# 4. 检查 ~/.zshrc 是否已 source
echo "[4/4] 检查 ~/.zshrc 配置..."
ZSHRC="$HOME/.zshrc"
SOURCE_LINE="[[ -f ${SCRIPT_DIR}/claude.zsh ]] && source ${SCRIPT_DIR}/claude.zsh"
if grep -qF "claude-switch/claude.zsh" "$ZSHRC" 2>/dev/null; then
  echo "      ~/.zshrc 已包含 source 行，跳过"
else
  echo "" >> "$ZSHRC"
  echo "# Claude provider 切换" >> "$ZSHRC"
  echo "${SOURCE_LINE}" >> "$ZSHRC"
  echo "      已写入 ~/.zshrc"
fi

echo ""
echo "=== 初始化完成 ==="
echo "执行 'source ~/.zshrc' 或重开终端后生效"
echo "命令：cs use claude | cs use minimax | cs status"
