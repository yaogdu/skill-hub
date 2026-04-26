# 企业级 AI 资产管理与分发平台（SkillHub / SHUB）产品需求文档

| **属性** | **说明** |
| --- | --- |
| **项目名称** | SkillHub（内部代号：SHUB） |
| **版本** | v2.7 |
| **状态** | 已对齐架构共识稿 |
| **目标对象** | AI 平台团队、基础架构团队、后端研发团队、SRE、企业研发效能团队 |
| **关联文档** | `ARCHITECTURE.md`、`docs/shub-skill-frontmatter.schema.json`、`docs/shub-asset-manifest.schema.json`、`docs/shub-package-metadata.schema.json`、`docs/shub-state.schema.json` |

---

## 1. 产品定位与战略结论

### 1.1 一句话定位

**SHUB 不是一个面向公众的 Skill 商店，而是一个面向企业内部的 AI 资产供应链与分发治理层。**

它解决的不是“全行业如何统一 Skill 标准”，而是“企业内部如何把 Prompt、Agent、MCP 这类 AI 能力资产，以统一包规范完成发布、检索、安装、隔离运行、离线复用以及对第三方工具的适配”。

### 1.2 为什么这件事仍然值得做

尽管 OpenAI、Anthropic、Google、Kimi、智谱等厂商都没有形成一个真正跨平台、可移植、公开统一的 Skill 包管理体系，但这**不能直接说明这件事不值得做**。它更准确地说明：

1. **大模型厂商的主战场不在这里**：他们优先解决模型能力、工具调用、工作流编排、商业化计费，而不是做中立的企业资产分发基础设施。
2. **公开 Skill Marketplace 不是企业的首要痛点**：企业真正痛的是资产如何规范化、如何审计、如何落盘、如何离线复用、如何接入现有开发工具链。
3. **跨平台通用 Skill 标准过早统一难度很高**：Skill 往往同时耦合 Prompt、脚本、依赖、权限、运行环境和宿主工具，不适合一开始就追求行业标准化。
4. **企业内部存在明确机会窗口**：如果 SHUB 聚焦“内部 AI 资产治理 + 分发 + 兼容层”，它提供的是厂商中立能力，而不是与大模型厂商正面竞争。

### 1.3 产品边界判断

**值得做的方向：**

- 企业内部 Prompt / Agent / MCP 资产统一建模
- `SKILL.md` 兼容优先的统一包规范
- 统一发布流程、统一本地安装体验
- 本地运行环境隔离
- 离线使用与本地缓存
- 第三方工具适配（Codex、Claude Code、Cursor、Aider）
- 来源可信、版本可追踪、发布可审计

**不值得首版投入的方向：**

- 面向公众的开放 Skill Marketplace
- 试图定义全行业通用标准并要求外部生态直接兼容
- 复杂插件市场与生态抽成模式
- 一开始就把所有 Agent 工具做成原生深度集成

---

## 2. 已确认的关键决策

| **主题** | **结论** | **说明** |
| --- | --- | --- |
| 仓库与产品关系 | **1A：直接在当前仓库演进为 SHUB** | 复用现有 Registry / API / DB 基础，不拆多仓 |
| 资产模型 | **2A：统一 Asset 模型，旧 skill / prompt / agent 模型废弃** | Hub 后端统一为 `Asset`，通过 `category` 区分类型 |
| 包规范 | **Skill-first：作者主入口改为 `SKILL.md`** | 与现有 skill 生态兼容，`shub:` 扩展承载企业字段 |
| Codex 适配 | **5C：两种都支持，但 P0 优先 5A 扁平导出** | P0 先走 `~/.shub/exports/*.md` Shadow Mapping；P1 再适配原生 `~/.codex/skills` |
| CLI 分发 | **4B：Go CLI + npx wrapper** | 主逻辑用 Go，npm 只负责下载和转发二进制 |
| v1 企业能力边界 | **P0：核心分发与离线** | GitLab/CI 唯一写入、本地环境隔离、离线使用、Shadow Mapping |
| UI 定位 | **弱化 UI，强化 CLI** | UI 主要作为只读浏览与发现入口 |

---

## 3. 核心架构逻辑

1. **真理之源（Source of Truth）**：企业内部私有 GitLab。
2. **发布路径**：GitLab Tag / CI 是唯一写入入口，UI 不直接创建生产资产。
3. **中枢 Hub（Registry）**：负责资产索引、版本元数据、搜索、包引用和发布状态管理。
4. **分发客户端（shub CLI）**：负责解析 `SKILL.md`、下载、解压、环境构建、导出映射和离线状态维护。
5. **本地家目录（Local Home）**：负责缓存、版本隔离、运行时隔离、导出文件与修复状态。
6. **影子映射层（Shadow Mapping）**：将内部复杂目录结构导出成第三方工具可消费的稳定扁平路径。
7. **规范分层**：对外遵循 Skill 生态的 `SKILL.md` 形态；对内统一收敛为 SHUB 的 `Asset` 模型。

---

## 4. 统一资产模型与 `SKILL.md` 包规范

### 4.1 统一资产模型

SHUB 首版不再维护平行的 `skill`、`prompt`、`agent` 三套产品模型，而是统一收敛为一个 **Asset** 模型：

- `id`：资产唯一标识，如 `arch/java-analyzer`
- `category`：资产类别，取值为 `prompt`、`agent`、`mcp`
- `version`：SemVer 版本号
- `source_skill`：原始 `SKILL.md` 内容或解析结果
- `manifest`：Hub 内部归一化后的派生清单
- `source`：来源仓库、提交、构建产物等信息
- `status`：发布状态，如 `draft`、`published`、`deprecated`
- `published_at` / `updated_at`

### 4.2 包目录结构

每个 SHUB 资产包以 `SKILL.md` 为作者主入口：

```text
<asset-root>/
├── SKILL.md
├── bin/                # 可选
├── scripts/            # 可选
├── mcp-config.json     # 可选，主要用于 mcp 类资产
└── ... 其他文件
```

### 4.3 `SKILL.md` 规范分层

SHUB 采用两层规范：

1. **外层兼容层**：遵循现有 skill 生态可识别的 `SKILL.md` + frontmatter 结构。
2. **内层扩展层**：在 frontmatter 中使用 `shub:` 命名空间承载企业级字段。

首版以 `docs/shub-skill-frontmatter.schema.json` 作为**作者侧主规范**。

### 4.4 `SKILL.md` frontmatter 要求

**通用字段：**

- `name`
- `description`
- `version`
- `allowed-tools`（可选，按外部 skill 生态兼容）

**SHUB 扩展字段：**

- `shub.schemaVersion`
- `shub.id`
- `shub.category`
- `shub.entry`
- `shub.runtime`
- `shub.exports`（可选）
- `shub.hooks`（可选）
- `shub.metadata`（可选）

### 4.5 `SKILL.md` 与派生清单的关系

- `SKILL.md` 是作者编写、工具兼容和发布校验的**唯一主输入**。
- `docs/shub-asset-manifest.schema.json` 描述的是 SHUB Hub / CLI 内部使用的**归一化派生 manifest**，不再作为作者主规范。
- `shub lint`、`shub package`、`shub deploy` 都优先解析 `SKILL.md`，必要时生成内部 manifest 供索引、缓存和服务端处理。

### 4.6 类别语义

| **类别** | **主要可消费对象** | **运行时要求** |
| --- | --- | --- |
| `prompt` | `SKILL.md` 正文 | 默认无独立运行时 |
| `agent` | `SKILL.md` 正文 + 可执行逻辑 | 按需创建隔离环境 |
| `mcp` | `SKILL.md` frontmatter + `mcp-config.json` 或命令入口 | 视声明决定是否构建本地运行时 |

---

## 5. CLI 能力范围

### 5.1 用户侧核心命令

| **命令** | **目标** | **行为描述** |
| --- | --- | --- |
| `npx shub search <query>` | 搜索资产 | 通过名称、描述、标签与后续语义索引查找资产 |
| `npx shub add <asset-id>` | 安装资产 | 下载到 `~/.shub/hub`，必要时构建 `~/.shub/envs`；支持 `--fallback-source <name>` 在 Registry miss 时触发服务端镜像拉取 |
| `npx shub use <asset-id>@<version>` | 切换版本 | 更新本地选中版本与导出映射，不要求工具端改配置 |
| `npx shub sync` | 同步资产 | 依据本地游标增量拉取并刷新导出 |
| `npx shub doctor` | 诊断修复 | 检查锁、缓存、导出链接、运行时健康并尝试修复 |

### 5.2 发布侧核心命令

| **命令** | **目标** | **行为描述** |
| --- | --- | --- |
| `shub lint` | 校验规范 | 校验 `SKILL.md`、frontmatter、引用文件和安全规则 |
| `shub package` | 构建发布包 | 将 `SKILL.md` 资产打包为可发布制品 |
| `shub deploy` | 写入 Hub | 由 CI 调用，将制品与解析后的元数据发布到 Hub |

### 5.3 设计原则

- 用户面体验是 `npx shub ...`
- 实际业务逻辑由 Go 二进制完成
- 所有本地文件系统操作、进程锁、环境安装、导出映射都由 Go 负责
- 对外尽量兼容 skill 生态，对内保持统一 Asset 模型

---

## 6. 本地存储设计（Local Home Topology）

```text
~/.shub/
├── .lock
├── config.json
├── state.json
├── hub/
│   └── <registry_host>/<namespace>/<asset_name>/<version>/
├── envs/
│   └── <asset_hash>/
└── exports/
    ├── .metadata.json
    ├── <flattened-prompt>.md
    ├── <flattened-mcp>.json
    └── <tool-specific-file>
```

### 6.1 目录职责

- `config.json`：Registry 地址、Token、默认导出配置等
- `state.json`：已安装版本、当前选中版本、同步游标、运行时状态、导出状态
- `hub/`：原始资产缓存区，保持版本隔离
- `envs/`：按资产版本或哈希隔离的运行环境
- `exports/`：导出给第三方工具消费的稳定路径
- `.lock`：全局锁，避免并发安装/切换导致本地状态损坏

### 6.2 P0 约束

- 首版只要求 Linux / macOS 体验闭环
- 所有查找、切换、使用能力在资产已落地后必须支持离线
- 本地状态损坏时应优先通过 `doctor` 自愈

---

## 7. 第三方工具适配矩阵

| **目标工具** | **P0 适配方式** | **P1 方向** |
| --- | --- | --- |
| `npx skills` / skills-compatible clients | 直接消费 Git / 本地目录中的 `SKILL.md` | 强化发布地址兼容与镜像策略 |
| Codex CLI | 导出 `~/.shub/exports/*.md` 扁平 Prompt 文件 | 增加原生 `~/.codex/skills` 导出 |
| Claude Code | 导出 MCP 配置 / 命令配置并注入可消费路径 | 深化 Agent / MCP 双模式适配 |
| Cursor | 导出规则文件或项目级引用路径 | 更细粒度 workspace 集成 |
| Aider | 导出规则/配置引用文件 | 增强项目级自动注入 |

### 7.1 适配原则

- 不强依赖第三方工具私有内部目录结构
- 优先选择稳定、扁平、版本敏感度低的导出方式
- 作者侧优先兼容 `SKILL.md` 生态，分发侧再补 SHUB 的企业能力
- 使用导出层解决“内部复杂目录结构”与“外部工具只认单文件/固定路径”之间的矛盾

---

## 8. 企业级分发链路

### 8.1 发布流程（Publish）

1. 资产源码存放在企业私有 GitLab 仓库
2. Git Tag 或受控分支触发 GitLab CI
3. CI 执行 `shub lint`
4. CI 基于 `SKILL.md` 构建发布包 `shub package`
5. CI 调用 `shub deploy`
6. Hub 存储资产版本、来源信息、派生 manifest、包引用、搜索文档和发布状态

### 8.2 消费流程（Consume）

1. 用户运行 `npx shub search` 查找资产
2. 用户运行 `npx shub add <asset-id>` 安装资产
3. CLI 下载到 `~/.shub/hub/.../<version>`
4. 如资产声明运行时，则创建 `~/.shub/envs/<asset-hash>`
5. CLI 生成或刷新 `~/.shub/exports` 导出文件
6. 用户通过 Codex / Claude Code / Cursor / Aider / skills-compatible 工具直接消费导出结果

### 8.2.1 缺失资产的回源镜像

- 平台侧可在 Hub 后台维护若干 **命名 Source**，每条配置只要求：
  - `name`
  - `address`
- `address` 支持 GitHub / GitLab / Bitbucket 风格 tree URL，也支持 `{asset}`、`{name}`、`{version}` 占位符。
- 当用户执行 `npx shub add <asset-id> --fallback-source <name>` 且 Registry 中不存在该资产时：
  1. Hub 根据 Source 配置回源拉取远端资产目录
  2. 服务端重新打包为 SHUB `.tar.gz`
  3. 包写入 Hub 自己的存储目录
  4. 资产元数据同步发布回 Registry
  5. 客户端再按正常 Registry 分发路径完成安装

### 8.3 切换与修复流程

- `use`：只变更本地选中版本与导出映射
- `sync`：增量刷新元数据和本地缓存
- `doctor`：修复断裂链接、运行时损坏、状态不一致等问题

---

## 9. 非功能性需求（NFR）

### 9.1 安全性

- 发布路径必须由 GitLab/CI 统一控制，UI 只读
- P1 提供 Token 校验能力，并为 LDAP/RBAC 预留接口
- P1 在 `lint` / `deploy` 阶段接入基础静态审计能力
- 资产来源、版本、提交信息必须可追踪

### 9.2 鲁棒性

- 资产一旦落地，本地使用应尽量脱离网络运行
- Registry 不可用时，已安装资产仍可被查找、切换和使用
- 本地状态损坏时，CLI 需提供可恢复路径

### 9.3 兼容性

- P0 以 Linux / macOS 为交付边界
- P0 优先支持 Shadow Mapping，不以所有工具原生目录兼容为前提
- P0 的作者包规范优先兼容 `SKILL.md`
- P1 再扩展更深度的原生集成路径

### 9.4 可扩展性

- 统一 Asset 模型必须支持新增 category，而不再复制产品线逻辑
- `shub:` 扩展必须允许未来增加 hooks、exports、runtime 类型
- P2 再考虑插件机制，不阻塞 P0 主链路落地

---

## 10. 首版成功标准

首版不以“公有生态规模”衡量成功，而以“企业内部闭环”衡量成功：

1. **发布闭环成功**：团队能够通过 GitLab/CI 稳定发布基于 `SKILL.md` 的资产到 Hub。
2. **安装闭环成功**：用户能够通过 `npx shub add` 完成拉取、落盘和必要环境构建。
3. **使用闭环成功**：至少能稳定适配 Codex 扁平 Prompt 导出场景，并保留对 `SKILL.md` 生态的直接兼容能力。
4. **离线闭环成功**：资产安装后，在断网情况下仍可继续查找、切换和使用。
5. **运维闭环成功**：本地状态损坏时，`doctor` 能完成核心修复。

---

## 11. 当前明确非目标

以下内容不纳入首版交付承诺：

- 公开的 Skill Marketplace
- 对外宣称的行业统一 Skill 标准
- 全部第三方工具的原生深度适配
- 完整插件生态
- Windows 链接兼容闭环
- 企业级完整 LDAP / RBAC 闭环

---

## 12. 待后续细化项

以下问题已不阻塞 P0 开工，但需要在实现阶段继续细化：

1. `state.json` 的精确字段定义与演进策略
2. GitLab CI 的制品命名规则与元数据契约
3. Token 结构与 P1 权限边界
4. 原生 `~/.codex/skills` 导出策略
5. SHUB 对外发布地址与 `npx skills add <git-url>` 的映射策略
6. Windows 平台的链接回退方案
7. 本地 Registry miss 时的外部源联邦查询 / 回源缓存策略
8. 外部源查询的信任边界、缓存失效、优先级合并与权限模型

---

## 13. 结论

本项目的正确方向不是“做一个大而全的公开 Skill 平台”，而是“把企业内部 AI 资产从零散文件提升为可发布、可检索、可安装、可隔离、可离线、可适配的标准包”。

因此，**SHUB 的产品价值不建立在行业是否已经普遍采用统一 Skill 标准之上，而建立在企业内部 AI 资产治理是否存在真实工程痛点之上。**

在规范层面，当前结论是：**对外跟随 `SKILL.md` 生态，对内统一为 `Asset` 模型，用 `shub:` 扩展补齐企业能力。**
