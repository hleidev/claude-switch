# claude-switch (`cs`)

[English](README.md)

逐终端的 Claude Code 服务商切换器。一条命令在 Claude.ai(OAuth)与第三方 API
(MiniMax、DeepSeek…)之间切换——**每个终端窗口互相独立**,这是原生
`settings.json` 做不到的。

PATH 上的二进制是 `claude-switch`;日常用安装器写入的短命令 `cs`(一个 shell
函数)来驱动——因为环境变量注入必须发生在你的 shell 里,而不是子进程中。

## 原理

Claude Code 按环境变量选择后端:

- `ANTHROPIC_AUTH_TOKEN` 已设置 → API key 模式(走 `ANTHROPIC_BASE_URL`)
- `ANTHROPIC_AUTH_TOKEN` 未设置 → OAuth 模式(读 `~/.claude/.credentials.json`)

`cs use <provider>` 把某服务商的 env 注入当前 shell;`cs use claude` 清空并回落
OAuth。切换只影响该终端里**新启动**的 `claude`。

## 依赖

- **Go**(用于编译)—— `brew install go`
- **zsh 或 bash** —— 交互式 shell 集成

## 安装

```bash
git clone https://github.com/hleidev/claude-switch.git
cd claude-switch
make install          # 编译、装到 ~/.local/bin、写入 shell 集成
exec $SHELL           # 或开一个新终端
```

可像 autotools 项目一样改路径:`make install PREFIX=/usr/local`。

## 从旧 bash 版本迁移

若你用过之前未版本化的 bash `cs`(数据在 `~/.claude-switch/`):

```bash
cs migrate
```

它会导入你的服务商和默认值、把密钥重新注册到 Claude Code,并询问是否移除旧的
shell 集成。旧的 `~/.claude-switch/` 会原样保留,由你自行删除。

## 命令

| 命令 | 作用 |
|---|---|
| `cs add [provider]` | 添加服务商(交互选单 + 隐藏输入密钥;`--key-stdin` 可脚本化) |
| `cs use <provider>` | 把当前终端切到某服务商 |
| `cs use claude` | 当前终端重置回 Claude.ai(OAuth) |
| `cs default [provider]` | 查看 / 设置新终端默认加载的服务商 |
| `cs set <p> <字段> [值]` | 改单个字段(`cs set <p> key` 隐藏输入 API key) |
| `cs unset <p> <字段>` | 清除单个字段 |
| `cs list` | 列出服务商(✓ 默认,● 当前终端) |
| `cs status` | 当前终端的服务商与配置摘要 |
| `cs edit [provider]` | 在 `$EDITOR` 打开整个配置 |
| `cs remove <provider>` | 删除服务商 |
| `cs doctor` | 自检 |
| `cs migrate` | 从旧 `~/.claude-switch` 布局导入 |
| `cs version` | 输出版本号 |

内置预设:`minimax`、`deepseek`、`anthropic`。其余走 `custom…`,你自己填 base URL。

## 配置

单文件 `${XDG_CONFIG_HOME:-~/.config}/claude-switch/config.toml`(`0600`),密钥内联。
用 `cs edit` 直接编辑,或用 `cs set` 逐字段改。`cs list` / `cs status` 不会打印密钥。

## 卸载

```bash
make uninstall        # 移除 shell 集成(询问是否删配置)+ 删二进制
```

## 开发

```bash
make build            # -> bin/claude-switch
make test             # go test ./...
make fmt vet
```
