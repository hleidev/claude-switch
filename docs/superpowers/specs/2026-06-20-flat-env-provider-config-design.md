# 设计:provider 配置改为「一张扁平环境变量表」+ preset 运行时解析

日期:2026-06-20
状态:已与用户确认,待评审

## 背景与动机

当前 `cs add <preset>` 把 preset 的所有字段(base_url、model、分级模型、env)**快照**进用户的
`config.toml`。这带来两个问题:

1. **更新项目模型不会生效。** 用户已 `cs add` 的 provider,模型名已写死在用户文件里;项目更新
   `presets.toml` 后,用户仍是旧模型,等于「既要改项目、又要改用户」。
2. **配置概念割裂。** provider 同时存在「带类型字段」(`base_url`/`model`/…,翻译成固定的
   `ANTHROPIC_*` 变量名)和嵌套的 `[providers.x.env]` 表(任意透传变量)。但两者本质相同——
   都是要 `export` 进 shell 的环境变量。用户不理解为何 env 要单独拆成一个嵌套子表。

## 目标

- 用户文件里 preset provider **只存密钥**;模型/超时等默认值由项目模板维护,`cs use` 时动态套用。
- 项目更新模板 → `make install` → 用户 `cs use` 自动用上新值,**单点维护**。
- 取消「类型字段 vs env 表」的割裂:**一个 provider = 一张扁平的环境变量表**。

## 非目标

- 不改变 `cs use` 的逐终端注入机制、OAuth 回落(`claude`)语义、密钥注册进 `~/.claude.json` 的行为。
- 不为「自定义 provider」引入 preset(custom 无模板,配置仍须自带 `ANTHROPIC_BASE_URL` 等)。
- 不做模型校验、不联网拉取模型清单。

## 核心数据模型

一个 provider 就是 `map[string]string`,键是**真实环境变量名**,值是要导出的值。三个来源按优先级合并:

```
[defaults]  →  preset[name](项目模板)  →  config.providers[name](用户覆盖)  →  CLAUDE_SWITCH_PROVIDER
后者覆盖前者(逐键),空值表示「不导出/抹掉继承项」。
```

### 项目模板 `internal/presets/data/presets.toml`(扁平)

```toml
[glm]
ANTHROPIC_BASE_URL = "https://open.bigmodel.cn/api/anthropic"
ANTHROPIC_MODEL = "glm-5.2"
ANTHROPIC_DEFAULT_HAIKU_MODEL = "glm-4.7"
ANTHROPIC_DEFAULT_SONNET_MODEL = "glm-5.2[1m]"
ANTHROPIC_DEFAULT_OPUS_MODEL = "glm-5.2[1m]"
API_TIMEOUT_MS = "3000000"
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = "1"
CLAUDE_CODE_AUTO_COMPACT_WINDOW = "1000000"
```

### 用户文件 `~/.config/claude-switch/config.toml`(只放密钥 + 可选覆盖)

```toml
version = 2
default_provider = "glm"

[providers.glm]
ANTHROPIC_AUTH_TOKEN = "sk-..."
# 想覆盖才写,键名就是真实变量名:
# ANTHROPIC_MODEL = "glm-4.7"
```

### 特殊键

只有一个键需要特殊对待:`ANTHROPIC_AUTH_TOKEN`(密钥)。

- `cs add` / `cs set <p> key` 仍只采集它(隐藏输入),写进配置,并注册进 `~/.claude.json`。
- `cs list` / `cs status` 对它打码,不打印明文。
- 它的有无决定 API-key 模式 vs OAuth(`claude` provider 即「无此键」),语义不变。

`ANTHROPIC_BASE_URL` 不再是结构上的特殊字段,但连通性探测会读取合并后的它。

## 组件改动

### `internal/config`(schema + 合并 + 路径)

- **Config 结构**
  - `Providers map[string]map[string]string`(原 `map[string]Provider`)
  - `Defaults map[string]string`(原 `Defaults{ Env map[string]string }`,去掉嵌套,扁平化)
  - `Version`、`DefaultProvider` 不变。
  - 删除 `Provider` 结构体及其类型字段。
- **`BuildEnv` 改签名为 `BuildEnv(name string, preset map[string]string)`**,以**注入** preset 避免
  `config ↔ presets` 形成 import 环(presets 依赖 config)。合并顺序:`defaults` → `preset` →
  `config.providers[name]`,逐键覆盖;跳过非法键名(`ValidEnvKey`)与**空值**(空值=不导出);
  输出按键名排序保证确定性,末尾追加 `CLAUDE_SWITCH_PROVIDER=name`。
- **`SetField`/`UnsetField`(path.go)**:键即真实环境变量名,用 `ValidEnvKey` 校验;删除
  `typedSetters` 与 `env.` 前缀分支。`merge.go` 的 `orderedTypedFields` 删除。
- **常量**:新增 `AuthTokenKey = "ANTHROPIC_AUTH_TOKEN"`、`BaseURLKey = "ANTHROPIC_BASE_URL"`。

### schema 迁移 v1 → v2(`internal/config/schema.go`)

`CurrentSchemaVersion` 升到 2,新增 `migrateV1ToV2(tree)`(在解码后的 TOML tree 上操作):

- 每个 `providers.<name>`:类型键改名 + 摊平 `env` 子表
  - `base_url→ANTHROPIC_BASE_URL`、`auth_token→ANTHROPIC_AUTH_TOKEN`、`model→ANTHROPIC_MODEL`、
    `small_fast_model→ANTHROPIC_SMALL_FAST_MODEL`、`sonnet_model→ANTHROPIC_DEFAULT_SONNET_MODEL`、
    `opus_model→ANTHROPIC_DEFAULT_OPUS_MODEL`、`haiku_model→ANTHROPIC_DEFAULT_HAIKU_MODEL`
  - `env.{K}` 提升为顶层 `{K}`;丢弃空值。
- `defaults.env.{K}` 提升为 `defaults.{K}`。

迁移在 `config.Load` 时自动跑一次并备份 `.bak`(沿用现有机制)。

**preset 等值最小化(`minimizeProviders`)**:`config.Load` 在 schema 迁移之后再跑一步——对每个
匹配到 preset 的 provider,删除「值与 preset 默认完全相同」的变量(密钥永远保留),只留密钥 + 真正
不同的覆盖项。因此老用户升级后配置会自动收敛成最简形式,并随 preset 更新自动跟随;与 preset 不同的
值视为有意覆盖,保留不动。preset 解析通过 `config.SetPresetLookup` 注入(cmd 层用 `presets.Lookup`
注册),保持 config 包不直接依赖 presets,便于测试。改动若有发生则持久化(schema 升级时才额外写 `.bak`)。

### `internal/presets`

- `presets map[string]map[string]string`;`Lookup(name) (map[string]string, bool)`;`Names()` 不变。
- `presets.toml` 改写为扁平表(见上)。

### `internal/shellenv`

- `ForProvider(c, name)`:内部 `presets.Lookup(name)` 取模板,调用 `c.BuildEnv(name, preset)`。
  shellenv 可同时依赖 config 与 presets(presets 不依赖 shellenv,无环)。其余生成 `export` 逻辑不变。

### `cmd/add.go`

- preset:只采集密钥,写 `{ ANTHROPIC_AUTH_TOKEN: key }`。不再弹模型框、不写 base_url/model。
- custom:交互依次问 provider 名、`ANTHROPIC_BASE_URL`、密钥;写这两个键。更多变量交给 `cs set`/`cs edit`。
- 连通性探测:合并 preset+覆盖后取 `ANTHROPIC_BASE_URL` 探测;为空则提示跳过。
- flags:保留 `--key-stdin`、`--base-url`(custom 非交互时写 `ANTHROPIC_BASE_URL`)、`--force`、
  `--no-verify`;**删除 `--model`**(模型由模板维护,需要时 `cs set`)。

### `cmd/set.go`

- `key`/`auth_token` 仍走隐藏输入,写 `ANTHROPIC_AUTH_TOKEN`。
- 其余 `cs set <p> <ENV_NAME> <value>`,`<ENV_NAME>` 为真实变量名,经 `ValidEnvKey` 校验。
- 帮助文案与 `Long` 更新。

### `cmd/list.go` / `cmd/status.go`

- list:展示合并后的 `ANTHROPIC_BASE_URL` 与「key set / no key」(看配置里是否含 `ANTHROPIC_AUTH_TOKEN`)。
- status:沿用读取实时进程 env 的方式(不变),措辞微调即可。

### `cmd/doctor.go`

- 新增检查:某 provider 在配置里存在,但**既无对应 preset、合并后又无 `ANTHROPIC_BASE_URL`** →
  报告「无法解析 base_url」。

### `internal/migrate`(bash 时代导入)

- `ToProvider` 简化:解析出的 `vars` 经 `ValidEnvKey` 过滤后**直接作为扁平表**返回(保留 `${VAR}` 展开
  与 holder 去重逻辑),不再映射到类型字段。`cs migrate` 写出 v2 扁平格式。

## 数据流(`cs use glm`)

```
config.Load() ──► providers.glm = { ANTHROPIC_AUTH_TOKEN: "sk-..." }
presets.Lookup("glm") ──► { ANTHROPIC_BASE_URL, ANTHROPIC_MODEL, 分级模型, API_TIMEOUT_MS, ... }
BuildEnv("glm", preset):
   defaults → preset → providers.glm 覆盖 → 排序 → 末尾 CLAUDE_SWITCH_PROVIDER=glm
shellenv 生成 export ... ──► eval 进当前 shell
```

custom provider(无 preset):`preset = nil`,只用 defaults + 用户配置。

## 错误处理

- provider 不存在 → 现有错误不变。
- `claude` 仍拒绝被当作可配置 provider。
- 合并后无 `ANTHROPIC_BASE_URL`(custom 漏填,或 preset 被新二进制删除而用户只剩密钥)→ `cs use`/探测
  给出明确报错指引(`cs set <p> ANTHROPIC_BASE_URL ...`),`cs doctor` 也会提示。
- 非法环境变量键名 → 写入时 `SetField` 报错;合并/生成阶段防御性跳过。
- 配置 schema 版本高于本二进制 → 沿用现有「请升级」报错。

## 测试

- `merge_test`:defaults/preset/覆盖三层合并、空值抹除、非法键跳过、排序确定性、`CLAUDE_SWITCH_PROVIDER` 末位。
- `schema_test`:v1→v2 迁移(类型键改名、env 摊平、defaults 摊平、空值丢弃),幂等。
- `path_test`:`SetField`/`UnsetField` 用真实键名 + 校验。
- `add_test`:preset 只写密钥;custom 写 base_url+密钥;`--key-stdin` 路径。
- `presets_test`:扁平 Lookup。
- `shellenv_test`:preset 注入后的 export 输出。
- `migrate_test`:bash 导入产出扁平 v2。

## 文档

- `README.md` / `README.zh.md`:更新配置格式示例(扁平、只放密钥)、`cs set` 用真实变量名、`cs add`
  只问密钥;保留已加的「更新/Updating」小节(本设计正是其前提)。

## 取舍与已确认决定

- **放弃类型字段的友好简称**(`cs set glm model` → `cs set glm ANTHROPIC_MODEL`),只保留 `key`/
  `auth_token` 作为密钥别名。用户已确认接受,换取「一张扁平表」的统一心智模型。
- **保留 preset 内部用真实变量名扁平书写**,不再有嵌套 `[env]`。
- 老配置经迁移后**值保留为覆盖**,不自动清空(可预期、不丢用户自定义);自动跟随更新需用户主动精简。
```
