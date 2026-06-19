# claude-switch（cs）Go 重写设计文档

- 日期：2026-06-17
- 分支：`feat/go-rewrite`
- 状态：设计稿，待评审

## 1. 背景与目标

当前项目是一个 Claude Code 的"逐终端服务商切换器"：用 shell 环境变量决定 Claude Code 走原生订阅（OAuth）还是第三方 API（MiniMax、DeepSeek…），每个终端窗口互相独立。

现状有两个根本问题：

1. **它是一个"项目"而非"工具"**：要 clone 仓库、装依赖、跑脚本。
2. **服务商定义硬编码在脚本源码里**（bash 函数 `_template_*`/`_defaults_*`），加一个新服务商必须改脚本本身，且 `add` 只会把模板甩进 `$EDITOR` 让人手填。

本次重写的目标：

- **成为一个真正的工具**：Go 编译的单二进制，`brew install` 即用，零运行时依赖。
- **配置即数据**：服务商定义放进一个用户可读、工具可写的配置文件，加服务商不再改代码。
- **录入交互主流化**：预设目录 + 引导式录入 + 可脚本化旁路，不再强制手写文件。
- **保住唯一不可替代的能力**：逐终端隔离（原生 `settings.json` 和所有插件都做不到）。

### 不在本次范围（明确非目标）

- Windows 支持（用户单人 macOS 使用；Linux 顺带支持）。
- macOS Keychain 存密钥（先用文件 + 0600；keychain 列为后续）。
- fish/其它 shell（首发支持 zsh + bash）。
- 代理/路由层（不做请求拦截，只做 env 注入）。

## 2. 形态与技术栈

- **语言**：Go。
- **CLI 框架**：cobra（子命令、help、shell 补全、动态补全一站式；gh/kubectl 同款）。
- **配置解析**：`pelletier/go-toml/v2`。
- **分发**：GoReleaser → 个人 Homebrew tap + GitHub Releases；打 git tag 由 GitHub Actions 自动出 macOS(arm64/amd64) + Linux 二进制并更新 formula。
- **命名**：
  - PATH 上的真二进制：**`claude-switch`**（唯一、不撞名）。
  - 用户日常敲的命令：**`cs`**，由 shell 集成里的 `cs()` 函数提供（该函数本就为 env 注入而存在），内部调用 `command claude-switch`。
  - 文档以 `claude-switch` 为正式名，`cs` 作为推荐别名介绍。
- 二进制内嵌版本号（`-ldflags`），供 `cs version` 输出。

> 命名理由：放上 PATH 的东西应唯一（避免与 Scala 生态的 coursier `cs` 冲突）；短名 `cs` 退化为用户自有的 shell 函数，遵循 kubectl→`k`、terraform→`tf` 的"规范长名 + 个人短别名"惯例。

## 3. 配置文件

### 3.1 位置

遵循 XDG：

- 配置文件：`${XDG_CONFIG_HOME:-~/.config}/claude-switch/config.toml`
- 权限：`0600`（单文件内联密钥，靠权限保护；kubectl/docker/gh/npm 同款单文件模式）。
- 目录不存在时由工具创建（`0700`）。

### 3.2 Schema

```toml
version = 1                        # 配置 schema 版本，迁移用
default_provider = "minimax"       # 新终端自动加载它；"claude" 表示不注入（回落 OAuth）

[defaults]                         # 可选：用户希望对所有 provider 全局套用的偏好
                                   # 出厂为空，绝不预置服务商专属参数
  [defaults.env]                   # 可选：全局透传 env

[providers.minimax]
base_url = "https://api.minimaxi.com/anthropic"   # 必填
model    = "MiniMax-M3"                            # → ANTHROPIC_MODEL
auth_token = "sk-..."                              # 密钥内联（文件 0600）
# 可选角色模型，留空则不输出对应变量：
# small_fast_model =                               # → ANTHROPIC_SMALL_FAST_MODEL
# sonnet_model     =                               # → ANTHROPIC_DEFAULT_SONNET_MODEL
# opus_model       =                               # → ANTHROPIC_DEFAULT_OPUS_MODEL
# haiku_model      =                               # → ANTHROPIC_DEFAULT_HAIKU_MODEL
  [providers.minimax.env]          # 服务商专属/任意透传 env（来自各家官方文档）
  API_TIMEOUT_MS = "3000000"
  CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = "1"
  CLAUDE_CODE_AUTO_COMPACT_WINDOW = "512000"

[providers.deepseek]
base_url    = "https://api.deepseek.com/anthropic"
model       = "deepseek-v4-pro"
haiku_model = "deepseek-v4-flash"
auth_token  = "sk-..."
  [providers.deepseek.env]
  CLAUDE_CODE_EFFORT_LEVEL   = "max"
  CLAUDE_CODE_SUBAGENT_MODEL = "deepseek-v4-flash"
```

### 3.3 字段到环境变量的映射

| 配置字段 | 环境变量 |
|---|---|
| `base_url` | `ANTHROPIC_BASE_URL` |
| `auth_token` | `ANTHROPIC_AUTH_TOKEN` |
| `model` | `ANTHROPIC_MODEL` |
| `small_fast_model` | `ANTHROPIC_SMALL_FAST_MODEL` |
| `sonnet_model` | `ANTHROPIC_DEFAULT_SONNET_MODEL` |
| `opus_model` | `ANTHROPIC_DEFAULT_OPUS_MODEL` |
| `haiku_model` | `ANTHROPIC_DEFAULT_HAIKU_MODEL` |
| `[providers.X.env]` 每个键 | 原样透传 |

- 留空的 typed 字段**不输出**对应环境变量（让 Claude Code 走默认）。
- 始终额外输出 `CLAUDE_SWITCH_PROVIDER=<name>`，供 `cs status` 报告当前终端身份。

### 3.4 合并优先级

注入时按以下顺序合并，后者覆盖前者：

`[defaults].env` → provider 的 typed 字段 → `[providers.X.env]`

### 3.5 参数归属原则（关键）

不存在"通用 timeout/disable 默认"。除真·通用核心（`base_url`/`auth_token`/`model` + 标准角色模型）外，所有参数都是**服务商专属**，存于该 provider 的 `[env]`，取值**逐家依官方 Claude Code 文档**填入预设，不臆造。已核实：

- MiniMax 官方：`API_TIMEOUT_MS=3000000`、`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`、`CLAUDE_CODE_AUTO_COMPACT_WINDOW=512000`（M3 上下文窗口）。
- DeepSeek 官方：`CLAUDE_CODE_EFFORT_LEVEL=max`、`CLAUDE_CODE_SUBAGENT_MODEL=deepseek-v4-flash`。

## 4. 内置预设

- 常见服务商（minimax、deepseek、glm、kimi、openrouter、anthropic…）的 `base_url`、默认 `model`、角色模型、专属 `[env]` 作为**数据**用 Go `embed` 编进二进制。
- `cs add` 选预设后，把预设字段写入 `config.toml`，只需用户补 `auth_token`；写入后完全可改。
- 每个预设的 `[env]` 内容以各服务商官方 Claude Code 文档为准。

### `claude` 与 `anthropic` 的区分

- **`claude`**：原生订阅（OAuth），是"重置态"——不进 `config.toml`，`cs use claude` 表示清空所有注入的 env、回落 `~/.claude/.credentials.json`。`add`/`edit`/`remove claude` 一律拒绝并给引导。
- **`anthropic`**：可选预设，指用真正的 `sk-ant-` 直连官方 API（`base_url=https://api.anthropic.com`），是一个普通 provider，与 OAuth 的 `claude` 是两回事。

## 5. Shell 集成与注入机制

### 5.1 命令分派

`cs()` 函数只对会改变 env 的子命令做特殊处理，其余透传给二进制：

- `cs use <p>` / 新终端自动加载：调用隐藏子命令 `claude-switch __shellenv <p>`，对其 stdout 做 `eval`。
- 其它（`add`/`list`/`set`/…）：直接 `command claude-switch "$@"`。

### 5.2 stdout/stderr 纪律（硬规则）

`__shellenv` 的 **stdout 只输出可被 eval 的 shell 代码**；所有人类可读信息（"已切换到 X"、告警）一律走 **stderr**。否则提示文字会被 eval 当命令执行。

### 5.3 干净切换：`_CS_MANAGED_VARS`

二进制是子进程，看不到当前 shell 的变量，因此清理逻辑必须在 shell 上下文执行。`__shellenv` 生成的代码形如：

```sh
for v in ${_CS_MANAGED_VARS}; do unset "$v"; done   # 用实时 shell 里的旧值
export ANTHROPIC_BASE_URL="https://api.deepseek.com/anthropic"
export ANTHROPIC_AUTH_TOKEN="..."
export ANTHROPIC_MODEL="deepseek-v4-pro"
export CLAUDE_CODE_EFFORT_LEVEL="max"
export CLAUDE_SWITCH_PROVIDER="deepseek"
export _CS_MANAGED_VARS="ANTHROPIC_BASE_URL ANTHROPIC_AUTH_TOKEN ANTHROPIC_MODEL CLAUDE_CODE_EFFORT_LEVEL CLAUDE_SWITCH_PROVIDER _CS_MANAGED_VARS"
```

- 先用上次记录的 `_CS_MANAGED_VARS` 精确 unset（含任意 `[env]` 透传变量），再 export 新集合，最后把本次集合写回 `_CS_MANAGED_VARS`。
- `cs use claude`：unset `_CS_MANAGED_VARS` 列出的全部变量及其自身，输出"已切换到 claude (OAuth)"到 stderr。

### 5.4 启动安全（硬红线）

新终端启动时执行的 `eval "$(claude-switch __shellenv <default>)"` **必须静默降级**：二进制不存在、配置损坏、默认 provider 缺失等任何错误都不得中断或污染 `.zshrc`/`.bashrc`。集成行带 `|| true` 兜底，二进制在异常时输出空 stdout + stderr 提示。

### 5.5 `cs init` / `cs setup` / 补全

- `cs init [zsh|bash]`：打印 shell 集成代码（定义 `cs()` 函数 + 启动自动加载 + 补全挂载）。不带参数时由 `$SHELL`/`$ZSH_VERSION`/`$BASH_VERSION` **自动识别当前 shell**。
- `cs setup`：一次性安装。自动识别 shell，**幂等地**把 `eval "$(command claude-switch init <shell>)"` 追加进正确的 rc 文件，并安装补全。
  - macOS 坑：bash 登录 shell 读 `~/.bash_profile` 而非 `~/.bashrc`，`setup` 需据此选择目标文件。
- `cs completion <zsh|bash>`：cobra 生成补全脚本（由 `setup` 自动安装）。`use`/`default`/`edit`/`remove`/`set` 的 provider 名用 cobra `ValidArgsFunction` **动态读 config 补全**。
  - 注意：cobra 默认按二进制名 `claude-switch` 注册补全，而用户敲的是 `cs()` 函数，init 脚本必须把补全**额外绑定到 `cs`**（zsh `compdef`，bash `complete`），否则 `cs use <Tab>` 不生效。

## 6. 命令清单与行为

| 命令 | 行为与边界 |
|---|---|
| `cs use <p>` | 切当前终端。无 arg → 错误 + usage。provider 不存在 → stderr 报错、非零、不改 env。provider 无 `auth_token` → 告警（避免静默回落 OAuth）。`cs use claude` → 重置。 |
| `cs add [p]` | 交互式：选预设/自定义 → 隐藏输入 key → 可选改 model → 写入 → 默认连通性探测（非阻断、可 `--no-verify`）→ 注册 key 到 `~/.claude.json` → 成功反馈 + 下一步。已存在 → 报错并指向 `edit`（或 `--force`）。非 TTY 且无 `--key`/`--key-stdin` → 明确报错。非交互旁路：`cs add <p> --key-stdin [--model … --base-url …]`。 |
| `cs edit [p]` | 打开 `$EDITOR` 编辑整个 `config.toml`（`git config -e` 风格，power-user 逃生口）。给 `p` 时校验其存在。`edit claude` 拒绝。保存后做 schema 校验，非法则报错并保留原文件。 |
| `cs set <p> <path> <value>` | 精准改单字段。裸名仅限 typed 字段（`base_url`/`model`/`*_model`）；其它一律 `env.<KEY>`；无法识别的裸名报错。`cs set <p> key` 特例：不收命令行参数，转隐藏输入（避免密钥进 history/ps）。 |
| `cs unset <p> <path>` | 删除某字段（typed 或 `env.<KEY>`）。 |
| `cs remove <p>` | 删除 provider，需确认（`--yes` 跳过）。同时清掉其在 config 中的整段。若它是当前默认 → 默认重置为 `claude`。`remove claude` 拒绝。 |
| `cs default [p]` | 无 arg 显示当前默认；设为不存在的 provider → 报错；设为 `claude` → 清除自动加载。 |
| `cs list` | 列出所有 provider，标注 ✓默认 与 ●当前终端（读 `CLAUDE_SWITCH_PROVIDER`），并标注 key 是否已设置。 |
| `cs status` | 当前终端 provider、base_url、token 是否设置（隐藏值）、model、全局默认、config 路径。 |
| `cs doctor` | 自检：shell 集成是否生效、默认 provider 是否有效、各 provider key 是否齐、配置能否解析、`~/.claude.json` 是否可写。给出修复建议。 |
| `cs migrate` | 从旧 `~/.claude-switch/` 迁移（见 §8）。 |
| `cs uninstall` | 移除 rc 里的集成行；询问是否删除 `config.toml`（含密钥）。对应旧 `uninstall.sh` 逻辑。 |
| `cs version` | 输出内嵌版本。 |
| `cs init` / `cs setup` / `cs completion` | 见 §5.5。 |
| `cs`（无参） | 打印帮助。 |

### 交互示例

`cs add`（交互）：

```
$ cs add
? 选择服务商:  (↑/↓ 选择，可输入过滤)
  ❯ minimax    MiniMax
    deepseek   DeepSeek
    glm        智谱 GLM
    kimi       Moonshot Kimi
    ──────────
    custom…    自定义新服务商
? 粘贴 API Key: ********************        (隐藏输入)
? 模型 [MiniMax-M3]: ↵                       (回车接受预设默认)
… 正在校验连通性 … ✓ 可达
✓ 已写入 'minimax' → ~/.config/claude-switch/config.toml
✓ 密钥已注册到 Claude Code (~/.claude.json)
  cs use minimax      切到此终端
  cs default minimax  设为新终端默认
```

`cs set`（非交互）：

```
$ cs set deepseek model deepseek-v4-pro
✓ deepseek.model = deepseek-v4-pro

$ cs set deepseek env.CLAUDE_CODE_EFFORT_LEVEL max
✓ deepseek.env.CLAUDE_CODE_EFFORT_LEVEL = max

$ cs set deepseek key
? 粘贴 API Key: ********
✓ 已更新 deepseek 的密钥
```

## 7. 安全与数据完整性

- `config.toml` 权限 `0600`；目录 `0700`。
- **写 `~/.claude.json` 必须原子写**（写临时文件 + `rename`），read-modify-write 时保留所有未知字段，只追加 key 尾号到 `customApiKeyResponses.approved`。写坏此文件会导致 Claude Code 无法启动，必须严防。
- 密钥录入优先隐藏输入；非交互场景提供 **`--key-stdin`**（从 stdin 读）作为安全旁路，避免 `--key` 明文进 shell history 与 `ps`。
- `cs status`/`list` 不打印密钥明文。

## 8. 版本与迁移（标准化）

### 8.1 两条独立的版本轴

业界惯例是把"程序版本"与"配置格式版本"分开，混用是反模式。

| 轴 | 形式 | 出处 | 何时变 |
|---|---|---|---|
| **程序版本** | SemVer（`0.2.0`） | git tag / GoReleaser，`-ldflags` 注入，`cs version` 输出 | 每次发布 |
| **配置 schema 版本** | 单调递增**整数**（`version = 1`） | `config.toml` 顶部 `version` 字段 | 仅当磁盘格式不兼容地改变时 +1 |

- 代码常量 `config.CurrentSchemaVersion`（当前 = 1）是本构建理解的格式版本。
- **`version` 字段缺失 ⇒ 视为 schema 版本 0**，即未版本化的 bash 旧时代。
- 参考：Terraform state 整数 `version`（与 Terraform semver 分离）、Kubernetes `apiVersion`、Docker Compose file version。

### 8.2 同位置 schema 升级（v1→v2→…，自动）

借鉴数据库迁移工具（Rails/Django migrations、Flyway、golang-migrate 的顺序步骤）：

- 注册一条有序升级链 `schemaMigrations = [{toVersion, apply(tree)} …]`，每个 step 把**原始 TOML 树**（`map[string]any`）从 N 升到 N+1（在树上操作而非结构体，便于重命名/重构字段）。
- `config.Load` 读取时：版本 < 当前 → **先备份**（`config.toml.bak`）→ 逐级跑迁移 → 写回；版本 > 当前 → **报错**（拒绝运行，避免回写时静默丢弃不认识的字段）；相等 → 不动。
- **新增一次未来迁移 = 追加一个 step + `CurrentSchemaVersion++`**，这是标准化的扩展点。今日升级链为空（仅 v1）。

### 8.3 旧 bash 数据导入（schema v0→v1，显式 `cs migrate`）

v0 数据在**不同位置且不同格式**（`~/.claude-switch/`，受 `CS_HOME` 覆盖），故作为独立的显式导入而非自动跑（自动从 shell 启动钩子跑会意外且不安全）。`cs migrate` 完整搬迁：

- **provider**：`providers/*.zsh` → 已知服务商套用新预设并以旧 `.zsh` 的值覆盖；未知服务商**尽力**解析所有 `export KEY=VALUE`，核心变量进 typed 字段、其余安全 key 进 `[env]`、非法 key 跳过，提示"请核对"。
- **默认值**：读旧 `config.toml` 的 `default_provider` → 写新默认（指向已导入的 provider 时）。
- **key 重注册**：每个导入 provider 的 token 尾号写回 `~/.claude.json`（原子写、保留未知字段）。
- **安全**：已存在新 config 时先备份为 `config.toml.pre-migrate.bak`；旧目录保留（不自动删）。
- **shell 集成**：检测旧 `~/.local/bin/cs` 符号链接与 rc 里指向它的旧 eval 行；**交互确认后**移除它们并装新集成（`installIntegration` 与 `cs setup` 共用），非交互时仅打印指引。

## 9. 已知局限（写入文档）

- **非交互/IDE 衍生 shell 拿不到 env**：Claude Code 自身的 Bash 工具、VSCode/JetBrains 插件起的 shell 通常不 source rc 文件，因此注入的 env 不生效。这是 env 方案的固有边界（竞品 ccs 同样有），本工具服务于"你在交互式终端里手敲 `claude`"的场景。`cs doctor` 会提示这一点。

## 10. 测试

- **Go 单元测试**：配置读写往返、合并优先级、字段→env 映射、`set`/`unset` 路径解析、迁移逻辑。
- **Golden 测试**：`__shellenv` 在各场景（首次、切换、切到 claude、缺字段）的输出固定比对。
- **Shell 集成冒烟测试**：用真实 zsh/bash 跑 `eval`，验证变量被正确 set/unset（沿用类似现有 bats 的思路），并验证"二进制报错不破坏 shell 启动"。
- 测试用隔离的临时 `XDG_CONFIG_HOME` 与 `HOME`，绝不触碰真实配置与 `~/.claude.json`。

## 11. 后续（明确推迟）

- macOS Keychain 存密钥。
- fish 支持。
- Windows 支持。
- 公开 tap / 多人使用时的命名与发布流程完善。
