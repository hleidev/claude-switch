# claude-switch

[English](README.md)

基于终端会话的 Claude Code 多 provider 切换工具。新开终端自动加载默认 provider，一条命令即可在当前终端切换到其他 provider，关闭终端后自动恢复默认。

## 文件结构

```
claude-switch/
├── claude.zsh                   # 主脚本，source 到 ~/.zshrc
├── claude.local.zsh.example     # 默认 provider 模板（复制为 claude.local.zsh）
├── claude.local.zsh             # 默认 provider 配置（gitignored）
├── providers/
│   ├── minimax.zsh.example      # MiniMax 配置模板
│   ├── minimax.zsh              # MiniMax 实际配置（gitignored）
│   ├── deepseek.zsh.example     # DeepSeek 配置模板
│   └── deepseek.zsh             # DeepSeek 实际配置（gitignored）
└── install.sh                   # 一次性初始化脚本
```

## 首次配置

**1. 创建 provider 配置文件**

```bash
cp providers/minimax.zsh.example providers/minimax.zsh
# 编辑 providers/minimax.zsh，填入 MINIMAX_API_KEY

cp providers/deepseek.zsh.example providers/deepseek.zsh
# 编辑 providers/deepseek.zsh，填入 DEEPSEEK_API_KEY
```

**2. 运行初始化脚本**

```bash
bash install.sh
```

脚本会自动完成：

- 将每个 provider 的 API key 注册到 Claude Code 审批列表
- 创建 `claude.local.zsh`（设置新终端的默认 provider）
- 检查 Claude (claude.ai) OAuth 登录状态，未登录则引导授权
- 将 source 行写入 `~/.zshrc`

**3. 重载 shell**

```bash
source ~/.zshrc
```

## 日常使用

| 命令 | 效果 |
|---|---|
| `cs use minimax` | 当前终端切换到 MiniMax |
| `cs use deepseek` | 当前终端切换到 DeepSeek |
| `cs use claude` | 当前终端切换到 Claude (claude.ai) |
| `cs status` | 查看当前 provider 及环境变量 |

- 新开终端自动加载 `claude.local.zsh` 中配置的**默认 provider**
- 不同终端窗口可以同时使用不同 provider，互不影响
- 切换仅对**新启动**的 `claude` 实例生效，已在运行的会话不受影响

## 工作原理

Claude Code 根据环境变量优先级选择后端：

- `ANTHROPIC_AUTH_TOKEN` 已设置 → API key 模式（使用 `ANTHROPIC_BASE_URL`）
- `ANTHROPIC_AUTH_TOKEN` 未设置 → OAuth 模式（读取 `~/.claude/.credentials.json`）

`cs use <provider>` 通过 source provider 配置文件来设置环境变量，`cs use claude` 则清除所有相关变量。每个终端拥有独立的环境，窗口之间完全隔离。

## 新增 Provider

以现有模板为基础进行复制：

```bash
# 创建模板文件（提交到 git，用于 Tab 补全）
cp providers/minimax.zsh.example providers/openai.zsh.example
# 编辑 providers/openai.zsh.example，修改 endpoint、模型名和 key 变量名

# 创建实际配置文件（gitignored，填入真实 key）
cp providers/openai.zsh.example providers/openai.zsh
# 编辑 providers/openai.zsh，填入你的 API key
```

然后注册 key：

```bash
bash install.sh
```

> Tab 补全读取 `providers/*.zsh.example` 来生成可选列表。如果只创建了 `.zsh` 文件而没有对应的 `.zsh.example`，该 provider 可以正常使用，但不会出现在 Tab 补全中。

## 在新机器上部署

```bash
git clone <repo> ~/Personal/claude-switch
cd ~/Personal/claude-switch
cp providers/minimax.zsh.example providers/minimax.zsh
# 填入 API key
bash install.sh
source ~/.zshrc
```
