# cs — Claude Code provider 切换工具

[English](README.md)

基于终端会话的 Claude Code provider 切换工具。一条命令即可在 Claude.ai（OAuth）和第三方 API provider（MiniMax、DeepSeek 等）之间切换。不同终端窗口互相独立。

## 工作原理

Claude Code 根据环境变量优先级选择后端：

- `ANTHROPIC_AUTH_TOKEN` 已设置 → API key 模式（使用 `ANTHROPIC_BASE_URL`）
- `ANTHROPIC_AUTH_TOKEN` 未设置 → OAuth 模式（读取 `~/.claude/.credentials.json`）

`cs use <provider>` 将 provider 配置 source 进当前 shell。`cs use claude` 清除所有相关变量，回退到 OAuth 模式。每个终端拥有独立环境，窗口之间完全隔离。

## 前置条件

- **zsh** — macOS Catalina+ 默认使用 zsh；可通过 `echo $SHELL` 确认
- **python3** — 用于注册 API key
  - macOS：`xcode-select --install`（含 python3）或 `brew install python3`
  - Linux：通常预装；否则 `apt install python3` / `dnf install python3`
- **claude CLI** — 需已安装 Claude Code 且 `claude` 命令在 PATH 中可用

## 安装

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
bash install.sh
source ~/.zshrc
```

`install.sh` 将 `cs` 复制到 `~/.local/bin/cs`，并在 `~/.zshrc` 中添加 shell 集成行。

## 首次配置

**添加 provider：**

```bash
cs add minimax    # 打开 $EDITOR 填入 API key
```

**设置默认 provider**（每个新终端自动加载）：

```bash
cs default minimax
```

**登录 Claude.ai**（OAuth 模式）：

```bash
cs add claude
```

## 日常使用

| 命令 | 效果 |
|---|---|
| `cs use minimax` | 当前终端切换到 MiniMax |
| `cs use deepseek` | 当前终端切换到 DeepSeek |
| `cs use claude` | 当前终端切换到 Claude.ai（OAuth） |
| `cs default minimax` | 设置 MiniMax 为新终端的默认 provider |
| `cs default` | 查看当前全局默认 |
| `cs list` | 列出已安装的 provider |
| `cs status` | 查看当前 provider 及环境变量 |
| `cs add minimax` | 添加 MiniMax provider（打开编辑器） |
| `cs edit minimax` | 编辑 MiniMax 配置（模型名、key 等） |
| `cs remove minimax` | 删除 MiniMax provider |

- 新开终端自动加载 `cs default` 设置的**全局默认 provider**
- 不同终端窗口可以同时使用不同 provider，互不影响
- 切换仅对**新启动**的 `claude` 实例生效

## Provider 配置位置

所有 provider 配置存储在 `~/.claude-switch/providers/`。项目仓库中不包含任何密钥。

## 添加 provider

内置模板：`minimax`、`deepseek`、`claude`

```bash
cs add minimax    # 创建 ~/.claude-switch/providers/minimax.zsh 并打开编辑器
```

更新 provider 配置（模型名升级、key 轮换等）：

```bash
cs edit minimax
```

## 在新机器上部署

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
bash install.sh
source ~/.zshrc
cs add minimax    # 重新填入 API key
cs default minimax
```

## 测试

```bash
brew install bats-core
bats test/
```

测试使用隔离的 `CS_HOME`（位于 `/tmp/cs-test-*`），不会触碰 `~/.claude-switch/`。临时目录在每个测试结束后自动清理。
