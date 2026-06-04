# claude-switch

Claude Code 多 Provider 切换方案。默认走 MiniMax，按需在当前终端切换至 Claude (claude.ai) 或其他 provider，关闭终端自动恢复默认。

## 文件说明

```
claude-switch/
├── claude.zsh                  # 主脚本，source 到 ~/.zshrc
├── claude.local.zsh.example    # token 模板，复制后填入真实值
├── claude.local.zsh            # 真实 token（gitignored，不进仓库）
└── setup.sh                    # 新机器一次性初始化脚本
```

## 首次配置

**1. 填入 token**

```bash
cp claude.local.zsh.example claude.local.zsh
# 编辑 claude.local.zsh，填入 MINIMAX_API_KEY
```

**2. 运行初始化脚本**

```bash
bash setup.sh
```

脚本会自动完成：
- 检查 token 是否已填写
- 注册 MiniMax key 到 Claude Code 审批列表
- 检查 Claude (claude.ai) OAuth 登录状态，未登录则引导完成授权
- 将 source 行写入 `~/.zshrc`

**3. 重载 shell**

```bash
source ~/.zshrc
```

## 日常使用

| 命令 | 效果 |
|---|---|
| `cs use minimax` | 当前终端切换到 MiniMax |
| `cs use claude` | 当前终端切换到 Claude (claude.ai) |
| `cs status` | 查看当前终端的 provider 状态 |

- 新开终端默认走 **MiniMax**
- 不同终端窗口可以同时使用不同 provider，互不影响
- 切换仅对**新启动**的 `claude` 实例生效，已在运行的会话不受影响

## 切换原理

两个 provider 使用不同的认证机制，Claude Code 按以下优先级选择：

- `ANTHROPIC_AUTH_TOKEN` 有值 → API key 模式（MiniMax）
- `ANTHROPIC_AUTH_TOKEN` 未设置 → OAuth 模式（Claude (claude.ai)，读 `~/.claude/.credentials.json`）

`cs use minimax` 设置 token，`cs use claude` 清除 token，两者天然隔离，共用同一个 `~/.claude` 配置目录。

## 新增 Provider

以 DeepSeek 为例，在 `claude.zsh` 的 `cs()` 函数中加一个 case 分支：

```bash
deepseek)
  export ANTHROPIC_BASE_URL="https://api.deepseek.com/anthropic"
  export ANTHROPIC_AUTH_TOKEN="${DEEPSEEK_API_KEY}"
  export ANTHROPIC_MODEL="deepseek-chat"
  unset ANTHROPIC_SMALL_FAST_MODEL
  unset ANTHROPIC_DEFAULT_SONNET_MODEL
  unset ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL
  echo "[claude-switch] 当前终端 → DeepSeek"
  ;;
```

在 `claude.local.zsh` 中加入：

```bash
export DEEPSEEK_API_KEY="your-deepseek-api-key-here"
```

之后 `cs use deepseek` 即可使用，无需任何额外初始化。

## 新机器部署

```bash
git clone <repo> ~/Personal/claude-switch
cp ~/Personal/claude-switch/claude.local.zsh.example ~/Personal/claude-switch/claude.local.zsh
# 填入 token
bash ~/Personal/claude-switch/setup.sh
source ~/.zshrc
```
