<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="img/skill-hub-logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="img/skill-hub-logo-light.svg">
    <img src="img/skill-hub-logo-light.svg" alt="skill-hub" width="500"/>
  </picture>
</p>

<p align="center">
  <a href="./README.md">English</a> · 简体中文
</p>

<p align="center">
  <a href="https://github.com/yaogdu/skill-hub/stargazers"><img src="https://img.shields.io/github/stars/yaogdu/skill-hub?style=social" alt="GitHub Stars"></a>
  &nbsp;
  <a href="https://discord.gg/HTYNjF2y2t"><img src="https://img.shields.io/discord/1435836734666707190?label=Discord&logo=discord&logoColor=white&color=5865F2" alt="Discord"></a>
  &nbsp;
  <a href="https://github.com/yaogdu/skill-hub/releases"><img src="https://img.shields.io/github/v/release/yaogdu/skill-hub?label=Release" alt="Release"></a>
  &nbsp;
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-green.svg" alt="License"></a>
  &nbsp;
  <a href="https://golang.org/doc/install"><img src="https://img.shields.io/badge/Go-1.25+-blue.svg" alt="Go Version"></a>
</p>

<p align="center">
  <a href="https://github.com/yaogdu/skill-hub">GitHub</a> · <a href="https://github.com/yaogdu/skill-hub/releases">Releases</a> · <a href="#quick-start-zh">快速开始</a> · <a href="#usage-zh">使用示例</a> · <a href="#documentation-map-zh">文档导航</a>
</p>

<p align="center">
  <strong>Agent 时代的企业级能力资产 Hub：</strong>统一发布、解析、分发和治理 MCP Server、Agent、Skill 与 Prompt。
</p>

---

## 30 秒理解

`skill-hub` 是一个面向企业内部的 AI 能力资产注册中心和分发中心。

它要解决的问题是：团队开始自研 Agent 之后，Prompt、Skill、MCP Server、Agent 配置会散落在 Git 仓库、脚本、文档、个人机器和 CI/CD 里，缺少统一的版本、权限、审计和复用入口。

你可以把它理解成 Agent 时代的内部 Nexus / 制品库：

- 平台团队把可信的 MCP、Skill、Prompt、Agent 资产发布到一个私有 registry
- 开发者通过 Web UI、`arctl` 或 `npx @yaogdu-skill-hub/shub` 查找、安装和切换固定版本
- CI/CD 在构建 Agent 时解析 `shub.dependencies`，生成 `shub.lock`，保证发布可复现
- 企业通过用户、API Key、fallback source 和包存储策略统一治理资产来源和使用方式

`skill-hub` 不负责运行 MCP Server 或 Agent。运行时部署、流量路由、灰度发布和环境编排仍然应该交给 Kubernetes、CI/CD、IDE MCP 配置、agentgateway 或企业已有平台。`skill-hub` 负责的是资产发布、版本解析、包存储、权限治理和客户端配置导出。

---

## 带来什么价值？

| 角色 | 价值 |
|---|---|
| 平台 / 架构团队 | 建立统一可信的 AI 能力资产目录，控制哪些 MCP、Skill、Prompt、Agent 可以被团队使用 |
| Agent 开发者 | 像依赖 jar 包一样声明和锁定 Agent 依赖，减少手工复制配置和脚本 |
| 业务研发团队 | 快速发现已经沉淀好的能力，按固定版本拉取和复用 |
| 安全 / 运维团队 | 用账号、API Key、私有存储、fallback source 和审计信息管理资产生命周期 |
| CI/CD 流水线 | 在构建和发布时自动解析依赖、校验 lockfile、发布 SHUB 包，提升可复现性 |

典型场景：

- 公司内部建设统一 Skill / Agent / MCP 资产中心
- 自研 Agent 需要依赖固定版本的 Prompt、MCP Server 或其他 Skill
- 希望把 GitHub / GitLab / 内部仓库里的 Skill 第一次使用时镜像到私有 registry
- 希望 Codex、Claude Code、Cursor、Aider 等工具从统一位置消费企业批准的能力
- 需要私有化部署、API Key、用户角色、可配置存储和可控上游来源

---

## 核心概念

### Asset

`skill-hub` 用统一的 Asset 模型管理 AI 能力资产，目前主要覆盖：

- `prompt`：可复用提示词、规范、操作手册
- `skill` / SHUB package：以 `SKILL.md` 为入口的能力包
- `mcp`：MCP Server 配置或 MCP 资产包
- `agent`：Agent 蓝图、依赖声明和配置资产

### SHUB Package

SHUB 包是 `skill-hub` 推荐的发布格式。一个包以 `SKILL.md` 为入口，frontmatter 中声明资产 ID、版本、类型、运行时、导出方式和依赖。

最小示例：

```md
---
name: java-analyzer
description: Analyze Java services and produce architecture guidance.
version: 1.2.0
allowed-tools:
  - Read
  - Grep
shub:
  schemaVersion: shub.skill/v1alpha1
  id: platform/java-analyzer
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
  dependencies:
    prompts:
      - platform/review-prompt@1.4.0
    mcps:
      - id: platform/postgres-mcp
        version: 2.0.1
        category: mcp
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
---
# Java Analyzer

Analyze Java services and produce architecture guidance.
```

依赖声明不写 registry 地址和用户名密码。CLI 会通过 `--registry-url` / `ARCTL_API_BASE_URL` / `SHUB_API_BASE_URL` 以及 `--registry-token` / `ARCTL_API_TOKEN` / `SHUB_API_TOKEN` 连接对应的私有 registry。

---

<a id="quick-start-zh"></a>
## 快速开始

### 方式一：本地从源码启动

适合本地体验、开发和 PoC。

前置依赖：

- Docker Desktop
- Docker Compose v2+
- Go 1.25+

```bash
git clone https://github.com/yaogdu/skill-hub.git
cd skill-hub
make run-docker
```

启动后访问：

- Dashboard: `http://localhost:12121`
- API: `http://localhost:12121/v0`

默认管理员账号：

- 用户名：`admin`
- 密码：`admin`

上传的 SHUB 包默认持久化到宿主机：

- `${HOME}/Documents/skill-storage`

### 方式二：Kubernetes / Helm 部署

适合私有化环境、团队共享环境和长期运行。

```bash
helm install skill-hub oci://ghcr.io/yaogdu/skill-hub/charts/agentregistry \
  --version 0.2.2 \
  --set config.jwtPrivateKey=$(openssl rand -hex 32)
```

默认 chart 会启动一个内置 PostgreSQL，适合评估环境。生产环境建议使用外部 PostgreSQL，并挂载持久化存储：

```bash
helm install skill-hub oci://ghcr.io/yaogdu/skill-hub/charts/agentregistry \
  --version 0.2.2 \
  --set config.jwtPrivateKey=$(openssl rand -hex 32) \
  --set database.postgres.bundled.enabled=false \
  --set database.postgres.url=postgres://<user>:<password>@<host>:5432/<dbname>
```

---

## 配置 CLI

安装 `arctl`：

```bash
curl -fsSL https://raw.githubusercontent.com/yaogdu/skill-hub/main/scripts/get-arctl | bash
arctl version
```

在 Dashboard 的 `Settings` 页面创建 API Key，然后配置环境变量。

zsh / bash：

```bash
export SHUB_API_BASE_URL=http://localhost:12121/v0
export SHUB_API_TOKEN=<your-api-key>

export ARCTL_API_BASE_URL=http://localhost:12121/v0
export ARCTL_API_TOKEN=<your-api-key>
```

fish：

```fish
set -gx SHUB_API_BASE_URL http://localhost:12121/v0
set -gx SHUB_API_TOKEN <your-api-key>
set -gx ARCTL_API_BASE_URL http://localhost:12121/v0
set -gx ARCTL_API_TOKEN <your-api-key>
```

之后可以使用：

- `arctl`：完整 CLI
- `npx @yaogdu-skill-hub/shub`：面向 SHUB 安装、搜索和使用的 npm wrapper

---

<a id="usage-zh"></a>
## 使用示例

仓库里已经带了可打包示例，位于 [`examples/shub/`](./examples/shub/)。

### 1. 校验并打包一个 Skill

```bash
arctl shub lint examples/shub/native-skill
arctl shub resolve examples/shub/native-skill
arctl shub package examples/shub/native-skill
```

默认会生成：

```text
examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

### 2. 发布到私有 registry

```bash
arctl shub deploy examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

`deploy` 是历史兼容命名，这里表示“发布 SHUB 包到 registry”，不是把 MCP Server 或 Agent 部署到 Docker / Kubernetes。

### 3. 搜索、安装和切换版本

```bash
npx @yaogdu-skill-hub/shub search native
npx @yaogdu-skill-hub/shub add examples/native-skill
npx @yaogdu-skill-hub/shub use examples/native-skill@1.0.0
npx @yaogdu-skill-hub/shub doctor
```

安装后的资产会落到本地 `~/.shub`，并根据 `SKILL.md` 的 `shub.exports` 导出给 Codex、Claude Code、Cursor 或 Aider 等工具消费。

### 4. 在 Agent / Skill 中声明依赖

在 `SKILL.md` 中声明固定版本依赖：

```yaml
shub:
  dependencies:
    prompts:
      - platform/review-prompt@1.4.0
    skills:
      - platform/java-analyzer@1.2.0
    mcps:
      - id: platform/postgres-mcp
        version: 2.0.1
        category: mcp
```

本地或 CI 中解析依赖：

```bash
arctl shub resolve ./agent-dir
arctl shub resolve ./agent-dir --check
```

`resolve` 会根据当前 registry 配置解析依赖并写入 `shub.lock`。`--check` 适合放进 CI，用来阻止 lockfile 过期的构建。

### 5. 使用 fallback source 回源

当 registry 本地没有某个 Skill 时，客户端可以从内置或管理员配置的 fallback source 回源。

```bash
# 使用 GitHub 相关内置来源
npx @yaogdu-skill-hub/shub add unfallenwill/supercoder -g

# 指定某个命名来源
npx @yaogdu-skill-hub/shub add arch/java-analyzer --fallback-source github-main
```

回源成功后，服务端会拉取远端内容、打包、存储并镜像发布到本地 registry。后续团队成员就可以直接从内部 registry 使用同一个资产。

---

## 推荐工作流

### 作者发布资产

```text
编写 SKILL.md
  -> arctl shub lint
  -> arctl shub resolve
  -> arctl shub package
  -> arctl shub deploy
  -> skill-hub registry 存储版本和包
```

### 使用者消费资产

```text
npx @yaogdu-skill-hub/shub search
  -> npx @yaogdu-skill-hub/shub add
  -> npx @yaogdu-skill-hub/shub use asset@version
  -> 导出到 Codex / Claude Code / Cursor / Aider
```

### CI/CD 集成

```text
提交 Agent / Skill 代码
  -> CI 执行 arctl shub lint
  -> CI 执行 arctl shub resolve --check
  -> CI 打包并发布 SHUB 包
  -> 下游环境按 shub.lock 拉取固定版本资产
```

---

## Dashboard 能做什么？

Dashboard 适合作为团队内部运营后台：

- 浏览 MCP Server、Agent、Skill、Prompt
- 管理用户和 API Key
- 管理 fallback sources
- 控制匿名读取与 API Key 校验策略
- 查看和维护 registry 中的资产

---

## 边界说明

`skill-hub` 是资产 registry，不是运行时平台。

它负责：

- 发布和存储 SHUB 包
- 维护资产元数据、版本和搜索索引
- 解析 `shub.dependencies` 并生成 `shub.lock`
- 管理用户、API Key 和 fallback source
- 导出配置给 Codex、Claude Code、Cursor、Aider 等工具

它不负责：

- 运行 MCP Server 或 Agent
- 替代 Kubernetes / Docker / agentgateway
- 承担线上流量路由、灰度发布或弹性伸缩
- 作为通用 OCI 镜像仓库或模型仓库

---

<a id="documentation-map-zh"></a>
## 文档导航

- [`README.md`](./README.md)：英文总览
- [`README.zh-CN.md`](./README.zh-CN.md)：中文总览和快速开始
- [`examples/shub/README.md`](./examples/shub/README.md)：可打包的 SHUB 示例
- [`npm/shub/README.md`](./npm/shub/README.md)：npm wrapper 使用说明
- [`ARCHITECTURE.md`](./ARCHITECTURE.md)：架构、Asset 模型和 `SKILL.md` 规范
- [`docs/shub-skill-frontmatter.schema.json`](./docs/shub-skill-frontmatter.schema.json)：`SKILL.md` frontmatter schema
- [`docs/shub-lock.schema.json`](./docs/shub-lock.schema.json)：`shub.lock` schema
- [`DEVELOPMENT.md`](./DEVELOPMENT.md)：本地开发说明
- [`RELEASING.md`](./RELEASING.md)：发布说明

---

## 与 agentregistry 的关系

本项目是基于上游 [`agentregistry`](https://github.com/agentregistry-dev/agentregistry) 的二次开发开源分支，保留 registry、CLI、Web UI 等基础能力，同时把产品形态进一步收敛到企业内部 Skill Hub、SHUB 包分发、私有化部署、API Key 鉴权和 fallback source 治理。

---

## License

Apache 2.0，见 [`LICENSE`](./LICENSE)。
