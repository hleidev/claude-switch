<div align="center">

# claude-switch (`cs`)

**为每个终端单独切换 Claude Code 后端 —— 一条命令，无需改动全局配置。**

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev)

[安装](#安装) · [快速开始](#快速开始) · [命令](#命令) · [English](README.md)

</div>

---

为每个终端单独切换 Claude Code 的后端。一条命令，在 Claude.ai（OAuth）与第三方 API
（MiniMax、DeepSeek、GLM 等）之间切换；每个终端窗口互相独立，一个终端的切换不会
影响其它终端。原生 `settings.json` 是全局的，做不到这一点。

PATH 上的二进制名为 `claude-switch`，日常通过安装器写入的 `cs` shell 函数来调用。
之所以用函数而非命令行参数，是因为环境变量必须注入到当前 shell，子进程改不了它。

## 原理

Claude Code 根据环境变量选择后端：

- `ANTHROPIC_AUTH_TOKEN` 已设置 → API key 模式（走 `ANTHROPIC_BASE_URL`）
- `ANTHROPIC_AUTH_TOKEN` 未设置 → OAuth 模式（读 `~/.claude/.credentials.json`）

`cs use <provider>` 把对应服务商的环境变量注入当前 shell；`cs use claude` 清空并
回落到 OAuth。切换只影响该终端里**新启动**的 `claude`。

## 功能

- 每个终端单独切换，不影响其它窗口，也不改动全局 `settings.json`
- 内置预设：minimax、deepseek、glm、anthropic，填入密钥即可使用
- `cs use claude` 回落到 Claude.ai 的 OAuth 登录
- 密钥存放在 `0600` 权限的配置文件中，`cs list` / `cs status` 不会打印
- 安装器自动配置好 zsh 和 bash 的 shell 集成

## 依赖

- **Go**（用于编译）—— `brew install go`
- **zsh 或 bash** —— 交互式 shell 集成

## 安装

```bash
git clone https://github.com/hleidev/claude-switch.git
cd claude-switch
make install          # 编译、安装到 ~/.local/bin、写入 shell 集成
exec $SHELL           # 或开一个新终端
```

可像 autotools 项目一样自定义安装路径：`make install PREFIX=/usr/local`。

## 快速开始

```bash
cs add                # 选择服务商，粘贴密钥
cs use glm            # 当前终端切到 GLM
cs list               # ✓ 默认，● 当前终端
claude                # ...使用选中的服务商
```

`cs use claude` 切回 OAuth。切换后只对当前终端新启动的 `claude` 生效，其它终端不受影响。

## 命令

| 命令 | 作用 |
|---|---|
| `cs add [provider]` | 添加服务商（交互选单 + 隐藏方式输入密钥；`--key-stdin` 便于脚本化，`--base-url` 用于自定义） |
| `cs use <provider>` | 把当前终端切换到某服务商 |
| `cs use claude` | 把当前终端重置回 Claude.ai（OAuth） |
| `cs default [provider]` | 查看 / 设置新终端默认加载的服务商 |
| `cs list` | 列出服务商（✓ 默认，● 当前终端） |
| `cs status` | 当前终端的服务商与配置摘要 |
| `cs edit [provider]` | 在 `$EDITOR` 中打开整个配置 |
| `cs remove <provider>` | 删除服务商 |
| `cs doctor` | 诊断配置 |
| `cs migrate` | 从旧版 `~/.claude-switch` 布局导入 |
| `cs version` | 输出版本号 |

内置预设：`minimax`、`deepseek`、`glm`、`anthropic`。其余均为 `custom…` 服务商，
需自行提供 base URL（`--base-url`）。

## 配置

配置集中在单个文件 `${XDG_CONFIG_HOME:-~/.config}/claude-switch/config.toml`
（`0600`）。每个服务商就是一张扁平的环境变量表，键名即真实的变量名。内置预设的
模型、超时等默认值由项目维护（见 `internal/presets/data/presets.toml`），因此预设
服务商通常只需填一项密钥：

```toml
version = 2
default_provider = "glm"

[providers.glm]
ANTHROPIC_AUTH_TOKEN = "sk-..."
# 想覆盖预设的某个变量，就写上那一行（优先级高于预设）：
# ANTHROPIC_MODEL = "glm-4.7"
```

`cs use` 会按 `defaults → 预设 → 你的覆盖` 合并后导出。自定义（非预设）服务商没有
模板，需自行提供 `ANTHROPIC_BASE_URL`。用 `cs edit` 直接编辑该文件。`cs list` /
`cs status` 不会打印密钥。

## 更新

预设（内置模型名、base URL 等）在**编译时**打包进二进制，因此更新不是改配置文件，
而是拉取代码后重新编译安装：

```bash
cd claude-switch
git pull
make install          # 重新编译，覆盖 ~/.local/bin/claude-switch
cs version            # 确认版本
```

已经 `cs add` 过的服务商不会自动跟随预设变化（模型名等已写入你的配置）。要把某个
服务商刷新到最新预设默认值，可运行 `cs add <provider> --force`（需重新输入密钥），
或用 `cs edit` 手动修改。

## 从旧版 bash 迁移

如果你用过此前未版本化的 bash `cs`（数据存放在 `~/.claude-switch/`）：

```bash
cs migrate
```

它会导入你的服务商和默认值、把密钥重新注册到 Claude Code，并询问是否移除旧的
shell 集成。旧的 `~/.claude-switch/` 会原样保留，由你自行删除。

## 卸载

```bash
make uninstall        # 移除 shell 集成（询问是否删配置）+ 删除二进制
```

## 开发

```bash
make build            # -> bin/claude-switch
make test             # go test ./...
make fmt vet
```

## 许可证

MIT，详见 [LICENSE](LICENSE)。
