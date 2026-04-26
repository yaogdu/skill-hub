<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="img/skill-hub-logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="img/skill-hub-logo-light.svg">
    <img src="img/skill-hub-logo-light.svg" alt="skill-hub" width="500"/>
  </picture>
</p>

<h1 align="center" style="font-size: 3em;">skill-hub</h1>
<h3 align="center">统一管理、分发和治理 MCP Server、Agent、Skill 与 Prompt 的私有化 Hub。</h3>

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
  <a href="https://github.com/yaogdu/skill-hub">GitHub</a> · <a href="https://github.com/yaogdu/skill-hub/releases">Releases</a> · <a href="#quick-start-zh">快速开始</a> · <a href="#documentation-map-zh">文档导航</a> · <a href="./README.md">English README</a> · <a href="https://discord.gg/HTYNjF2y2t">Discord</a>
</p>

`skill-hub` 是一个面向 MCP Server、AI Agent、Skill、Prompt 的统一注册中心与分发中心，重点服务于团队内部沉淀、私有化部署、统一治理和标准化消费。

本项目是基于上游 [`agentregistry`](https://github.com/agentregistry-dev/agentregistry) 的二次开发开源分支。我们保留了它在 registry、CLI、部署、Web UI 方面的核心能力，同时把产品形态进一步收敛到更适合企业内部场景的方向：

- 自托管 / 私有化部署优先
- 面向 skill 分发和 SHUB 工作流优化
- 增强登录、角色、API Key 与后台治理能力
- 支持从 GitHub / GitLab 等远端源回源拉取并镜像入库

如果你希望在公司内部搭建一个统一的 skill / agent 资产中心，让团队成员通过一个 Dashboard 和一套 CLI 来发布、查找、拉取、安装、同步和治理 AI 资产，`skill-hub` 就是为这个场景准备的。

---

## 这个项目解决什么问题？

在很多团队里，MCP Server、Agent、Skill、Prompt 往往散落在：

- GitHub / GitLab 仓库
- npm / PyPI / Docker Hub
- 团队文档和 IM 对话
- 每个开发者自己的本地目录

常见问题包括：

- 不知道哪个版本能用、哪个地址可信
- 新同学接手时，需要手动复制一堆脚本和配置
- Skill 存在于远端仓库里，但团队内部没有统一镜像和治理
- 不同 IDE / AI 客户端接入方式各不相同
- 发布、回滚、权限控制和审计都缺少统一入口

`skill-hub` 把这些能力统一到一个服务里：你可以把 Skill、Agent、MCP Server、Prompt 收敛到一个私有 registry 中，通过 UI 和 CLI 统一发布、发现、拉取、同步和管理。

---

## 与 agentregistry 的关系

本项目不是从零重写，而是基于 `agentregistry` 进行二开，目标是：

- 保留其原有 registry / CLI / Web UI / 部署模型
- 在用户体验上更聚焦于 Skill Hub 场景
- 更适合公司内部私有化、自托管和权限治理

当前 `skill-hub` 的定位可以理解为：

> 一个基于 `agentregistry` 演进而来的、面向企业内部 AI 资产治理与 Skill 分发的私有化版本。

如果你准备把它放到公司内部使用，推荐在文档、介绍和对外说明中明确：

- 上游来源：`agentregistry`
- 当前项目：`skill-hub`
- 当前目标：企业内部私有化部署、统一分发、统一治理

---

## 适用场景

`skill-hub` 特别适合下面这些场景：

- 公司内部搭建统一的 Skill / Agent / MCP 资产中心
- 团队希望通过 API Key、用户角色和后台管理页面统一治理发布权限
- 需要把 GitHub / GitLab 上的 Skill 在第一次拉取时自动同步到本地 registry
- 需要给 Codex、Claude Code、Cursor、Aider 等工具提供统一的 Skill 消费入口
- 需要一个可私有部署、可控存储路径、可自定义上游源的内部平台

---

## 核心能力

### 1. 统一资产注册与发现

支持把以下内容纳入统一 registry：

- MCP Servers
- AI Agents
- Skills / SHUB Packages
- Prompts

团队成员可以通过：

- Dashboard
- `arctl`
- `npx @yaogdu-skill-hub/shub`
- API

来查询、浏览和使用这些资产。

### 2. SHUB 工作流

支持标准的 SHUB 打包、发布、安装、同步流程：

```bash
arctl shub lint ./skills/ai-agent-learning-system
arctl shub package ./skills/ai-agent-learning-system
arctl shub deploy ./dist/ai-agent-learning-system-1.0.0.tar.gz

npx @yaogdu-skill-hub/shub search ai-agent
npx @yaogdu-skill-hub/shub add yaogdu/ai-agent-learning-system
npx @yaogdu-skill-hub/shub use yaogdu/ai-agent-learning-system@1.0.0
```

### 3. 远端回源与镜像入库

当 registry 本地没有某个 Skill 时，`shub add` 可以按照配置的 fallback source 去远端查找，例如：

- GitHub
- GitLab
- 内部代码托管平台

拉取成功后，服务端会：

1. 从远端拉取内容
2. 自动打包
3. 存储到 registry 管理的存储目录
4. 同步发布一份到本地 registry

这样同一个 Skill 第一次从远端拉取后，后续团队成员就可以直接从内部 registry 使用。

### 4. 登录、角色与 API Key

当前已经支持基础权限模型：

- 管理员
  - 默认账号密码：`admin` / `admin`
  - 可创建普通用户
  - 可管理所有资产
  - 可配置 fallback sources
  - 可控制 API Key 校验开关
- 普通用户
  - 可以查看所有内容
  - 只能修改和删除自己拥有的内容
  - 可以生成自己的 API Key

CLI 和 npm wrapper 都支持通过环境变量读取 API Key。

### 5. 私有化部署友好

当前项目对私有化部署做了明确支持：

- 支持 Docker Compose 启动
- 支持 Helm / Kubernetes 部署
- Skill 包存储目录可配置
- fallback source 可配置
- 是否启用匿名读取 / API Key 校验可配置
- 适合作为团队内部基础设施长期运行

---

<a id="quick-start-zh"></a>
## 快速开始

### 前置依赖

- Docker Desktop
- Docker Compose v2+

### 1) 构建服务镜像

```bash
docker build \
  -f docker/server.Dockerfile \
  -t localhost:5001/agentregistry-dev/agentregistry/server:dev \
  .
```

### 2) 启动服务

```bash
env VERSION=dev DOCKER_REGISTRY=localhost:5001 docker compose \
  -f internal/daemon/docker-compose.yml \
  up -d
```

默认 Dashboard 地址：

- `http://localhost:12121`

默认情况下，Compose 会把上传的 SHUB 包落到宿主机：

- `${HOME}/Documents/skill-storage`

容器内挂载路径为：

- `/var/lib/agentregistry/storage`

这套方式比较适合本地验证和内部 PoC；如果是正式私有化部署，建议挂载持久化磁盘或对象存储前置层。

---

## 首次登录

启动后：

1. 打开 `http://localhost:12121`
2. 使用默认管理员登录
3. 默认账号：`admin`
4. 默认密码：`admin`

登录后可以在 `Settings` 中管理：

- API Keys
- Users
- Fallback Sources
- API Key 校验开关

---

## CLI / npm Wrapper 配置

创建 API Key 后，可以配置环境变量：

### zsh / bash

```bash
export SHUB_API_BASE_URL=http://localhost:12121/v0
export SHUB_API_TOKEN=<your-api-key>

export ARCTL_API_BASE_URL=http://localhost:12121/v0
export ARCTL_API_TOKEN=<your-api-key>
```

### fish

```fish
set -gx SHUB_API_BASE_URL http://localhost:12121/v0
set -gx SHUB_API_TOKEN <your-api-key>
set -gx ARCTL_API_BASE_URL http://localhost:12121/v0
set -gx ARCTL_API_TOKEN <your-api-key>
```

这样你就可以同时使用：

- `arctl`
- `npx @yaogdu-skill-hub/shub`

访问同一个私有 registry。

---

## Fallback Source 机制

当你执行：

```bash
npx @yaogdu-skill-hub/shub add <asset-id>
```

如果本地 registry 没有该内容，客户端会自动尝试 fallback source。

当前支持两类来源：

- 内置来源
  - `github-direct`
  - `github-skills-main`
  - `github-plugin-skills-main`
  - `openai-skills`
  - `anthropic-skills`
- 管理员在后台配置的自定义来源

额外用法：

```bash
# 只走 GitHub 相关回源逻辑
npx @yaogdu-skill-hub/shub add unfallenwill/supercoder -g

# 指定某个命名来源
npx @yaogdu-skill-hub/shub add arch/java-analyzer --fallback-source github-main
```

这套机制适合“用户不确定 Skill 是否已经入库，但又希望直接尝试拉取”的场景。

---

## Dashboard 能做什么

当前 Dashboard 适合做团队内部运营后台，主要支持：

- 浏览 Skills / Agents / MCP Servers / Prompts
- 登录与鉴权
- API Key 管理
- 用户管理
- Fallback Source 管理
- 控制匿名读取与 API Key 校验策略

推荐的页面结构是：

- 左侧导航
- 右侧内容区
- Settings 下拆分独立页面进行管理

这更适合持续运维，而不是只做一次性演示。

---

<a id="documentation-map-zh"></a>
## 文档导航

- [`README.md`](./README.md)：英文版总览
- [`README.zh-CN.md`](./README.zh-CN.md)：中文版总览
- [`DEVELOPMENT.md`](./DEVELOPMENT.md)：本地开发说明
- [`ARCHITECTURE.md`](./ARCHITECTURE.md)：架构说明
- [`RELEASING.md`](./RELEASING.md)：发布说明
- [`examples/shub/README.md`](./examples/shub/README.md)：SHUB 示例
- [`npm/shub/README.md`](./npm/shub/README.md)：npm wrapper 使用说明
- [`CONTRIBUTING.md`](./CONTRIBUTING.md)：贡献指南

---

## 开源说明

当前仓库是 `skill-hub` 的开源主仓库，GitHub 仓库地址：

- `https://github.com/yaogdu/skill-hub`

这个项目目前没有单独官网，GitHub 仓库本身就是主要入口，包括：

- 源码
- Issue
- Release
- 使用说明
- 开源协作

如果你准备对外介绍本项目，建议统一用下面这段描述：

> skill-hub 是一个基于 agentregistry 二次开发的开源项目，主要面向企业内部私有化部署场景，用于统一管理、分发和治理 MCP Server、Agent、Skill 与 Prompt 等 AI 资产。

---

## License

Apache 2.0，见 [`LICENSE`](./LICENSE)。
