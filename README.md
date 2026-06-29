<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="img/skill-hub-logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="img/skill-hub-logo-light.svg">
    <img src="img/skill-hub-logo-light.svg" alt="skill-hub" width="500"/>
  </picture>
</p>

<p align="center">
  English · <a href="./README.zh-CN.md">简体中文</a>
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
  <a href="https://github.com/yaogdu/skill-hub">GitHub</a> · <a href="https://github.com/yaogdu/skill-hub/releases">Releases</a> · <a href="#quick-start">Quick Start</a> · <a href="#usage">Usage</a> · <a href="#documentation-map">Docs</a>
</p>

<p align="center">
  <strong>An enterprise hub for agent capabilities:</strong> publish, resolve, distribute, and govern MCP servers, agents, skills, and prompts.
</p>

---

## What is skill-hub?

`skill-hub` is a self-hosted registry and distribution hub for AI capability assets: MCP servers, agents, skills, and prompts.

It is built for teams that are starting to build internal agents and need a reliable way to manage the assets those agents depend on. Without a registry, prompts, skills, MCP server configs, and agent blueprints tend to spread across Git repos, scripts, docs, chat history, CI jobs, and individual laptops.

Think of `skill-hub` as an internal Nexus-style repository for the agent era:

- platform teams publish approved MCP, skill, prompt, and agent assets into a private registry
- developers discover, install, and switch pinned versions through the dashboard, `arctl`, or `npx @yaogdu-skill-hub/shub`
- CI/CD resolves `shub.dependencies` into `shub.lock` for reproducible agent builds
- organizations govern asset sources with users, API keys, fallback sources, and private package storage

`skill-hub` is not a runtime deployment platform. It does not deploy MCP servers or agents into Docker, Kubernetes, or agentgateway. Runtime execution, routing, rollout, and scaling should stay in Kubernetes, CI/CD, IDE MCP configuration, agentgateway, or your existing platform. `skill-hub` owns asset publishing, version resolution, package storage, access control, and client configuration export.

---

## Why use it?

| Audience | Value |
|---|---|
| Platform teams | Provide one trusted catalog for approved AI building blocks |
| Agent developers | Declare and lock agent dependencies instead of copying scripts and configs by hand |
| Application teams | Discover reusable capabilities and install pinned versions quickly |
| Security and ops teams | Control users, API keys, fallback sources, package storage, and asset lifecycle |
| CI/CD pipelines | Validate packages, resolve dependencies, check lockfiles, and publish reproducible SHUB artifacts |

Use `skill-hub` when you need:

- an internal Skill / Agent / MCP asset center
- private publishing with users, API keys, and dashboard governance
- fallback imports from GitHub, GitLab, Bitbucket, or internal source platforms
- fixed versions for prompts, skills, MCP assets, and agent dependencies
- exports for tools such as Codex, Claude Code, Cursor, and Aider
- Docker Compose or Helm-based private deployment

---

## Core concepts

### Asset

`skill-hub` uses a unified asset model for:

- `prompt`: reusable instructions, standards, playbooks, and templates
- `skill` / SHUB package: a `SKILL.md`-rooted capability package
- `mcp`: an MCP server config or MCP asset package
- `agent`: an agent blueprint with dependency and configuration metadata

### SHUB package

A SHUB package is the preferred publishing format. It is a directory rooted by `SKILL.md`; the frontmatter declares the asset ID, version, category, runtime, exports, and dependencies.

Minimal example:

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

Dependency declarations do not contain registry URLs or credentials. The CLI resolves them through `--registry-url` / `ARCTL_API_BASE_URL` / `SHUB_API_BASE_URL` and `--registry-token` / `ARCTL_API_TOKEN` / `SHUB_API_TOKEN`.

---

<a id="quick-start"></a>
## Quick Start

### Option 1: Run locally from source

Use this for local evaluation, development, and proof-of-concept work.

Prerequisites:

- Docker Desktop
- Docker Compose v2+
- Go 1.25+

```bash
git clone https://github.com/yaogdu/skill-hub.git
cd skill-hub
make run-docker
```

Open:

- Dashboard: `http://localhost:12121`
- API: `http://localhost:12121/v0`

Default administrator:

- Username: `admin`
- Password: `admin`

Uploaded SHUB packages are persisted under:

```text
${HOME}/Documents/skill-storage
```

### Option 2: Install on Kubernetes with Helm

Use this for shared, self-hosted environments.

```bash
helm install skill-hub oci://ghcr.io/yaogdu/skill-hub/charts/agentregistry \
  --version 0.2.2 \
  --set config.jwtPrivateKey=$(openssl rand -hex 32)
```

The default chart starts a bundled PostgreSQL instance for evaluation. For production, use an external PostgreSQL service and persistent package storage:

```bash
helm install skill-hub oci://ghcr.io/yaogdu/skill-hub/charts/agentregistry \
  --version 0.2.2 \
  --set config.jwtPrivateKey=$(openssl rand -hex 32) \
  --set database.postgres.bundled.enabled=false \
  --set database.postgres.url=postgres://<user>:<password>@<host>:5432/<dbname>
```

---

## Configure the CLI

Install `arctl`:

```bash
curl -fsSL https://raw.githubusercontent.com/yaogdu/skill-hub/main/scripts/get-arctl | bash
arctl version
```

Create an API key in the dashboard under `Settings`, then point the CLI at your registry.

zsh / bash:

```bash
export SHUB_API_BASE_URL=http://localhost:12121/v0
export SHUB_API_TOKEN=<your-api-key>

export ARCTL_API_BASE_URL=http://localhost:12121/v0
export ARCTL_API_TOKEN=<your-api-key>
```

fish:

```fish
set -gx SHUB_API_BASE_URL http://localhost:12121/v0
set -gx SHUB_API_TOKEN <your-api-key>
set -gx ARCTL_API_BASE_URL http://localhost:12121/v0
set -gx ARCTL_API_TOKEN <your-api-key>
```

You can then use:

- `arctl` for the full CLI
- `npx @yaogdu-skill-hub/shub` for SHUB search, install, use, sync, and doctor flows

---

<a id="usage"></a>
## Usage

Runnable examples live under [`examples/shub/`](./examples/shub/).

### 1. Validate and package a skill

```bash
arctl shub lint examples/shub/native-skill
arctl shub resolve examples/shub/native-skill
arctl shub package examples/shub/native-skill
```

This writes:

```text
examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

### 2. Publish it to your registry

```bash
arctl shub deploy examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

The command name is kept for SHUB compatibility. It publishes the package into the registry; it does not deploy an MCP server or agent runtime.

### 3. Search, install, and activate a version

```bash
npx @yaogdu-skill-hub/shub search native
npx @yaogdu-skill-hub/shub add examples/native-skill
npx @yaogdu-skill-hub/shub use examples/native-skill@1.0.0
npx @yaogdu-skill-hub/shub doctor
```

Installed assets are stored under `~/.shub` and exported according to `shub.exports` for tools such as Codex, Claude Code, Cursor, and Aider.

### 4. Resolve dependencies in CI

Declare pinned dependencies in `SKILL.md`:

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

Resolve or check the lockfile:

```bash
arctl shub resolve ./agent-dir
arctl shub resolve ./agent-dir --check
```

`resolve` writes `shub.lock`. `--check` is intended for CI gates that should fail when the committed lockfile is stale.

### 5. Import from fallback sources

When an asset is missing from your registry, the client can ask configured fallback sources to fetch it from GitHub, GitLab, Bitbucket, or an internal source platform.

```bash
# Use GitHub-oriented built-in sources
npx @yaogdu-skill-hub/shub add unfallenwill/supercoder -g

# Force a named source
npx @yaogdu-skill-hub/shub add arch/java-analyzer --fallback-source github-main
```

On a successful fallback import, the server fetches the remote asset, packages it, stores it, and mirrors it back into your registry for later internal use.

---

## Recommended workflows

### Authoring and publishing

```text
write SKILL.md
  -> arctl shub lint
  -> arctl shub resolve
  -> arctl shub package
  -> arctl shub deploy
  -> skill-hub stores the versioned package
```

### Consuming assets

```text
npx @yaogdu-skill-hub/shub search
  -> npx @yaogdu-skill-hub/shub add
  -> npx @yaogdu-skill-hub/shub use asset@version
  -> export to Codex / Claude Code / Cursor / Aider
```

### CI/CD

```text
commit agent or skill code
  -> CI runs arctl shub lint
  -> CI runs arctl shub resolve --check
  -> CI packages and publishes the SHUB artifact
  -> downstream environments pull pinned versions from shub.lock
```

---

## Dashboard

The dashboard is the team-facing operations UI:

- browse MCP servers, agents, skills, and prompts
- manage users and API keys
- manage fallback sources
- control anonymous reads and API-key validation
- inspect and maintain registry assets

---

## Runtime boundary

`skill-hub` is an asset registry, not a runtime platform.

It does:

- publish and store SHUB packages
- maintain asset metadata, versions, and search indexes
- resolve `shub.dependencies` into `shub.lock`
- manage users, API keys, and fallback sources
- export configuration for Codex, Claude Code, Cursor, Aider, and other clients

It does not:

- run MCP servers or agents
- replace Kubernetes, Docker, or agentgateway
- own production routing, rollout, autoscaling, or traffic policy
- act as a general OCI image registry or model registry

---

<a id="documentation-map"></a>
## Documentation Map

- [`README.zh-CN.md`](./README.zh-CN.md): Chinese overview and quick start
- [`examples/shub/README.md`](./examples/shub/README.md): runnable SHUB examples
- [`npm/shub/README.md`](./npm/shub/README.md): npm wrapper usage
- [`ARCHITECTURE.md`](./ARCHITECTURE.md): architecture, asset model, and `SKILL.md` package contract
- [`docs/shub-skill-frontmatter.schema.json`](./docs/shub-skill-frontmatter.schema.json): `SKILL.md` frontmatter schema
- [`docs/shub-lock.schema.json`](./docs/shub-lock.schema.json): `shub.lock` schema
- [`DEVELOPMENT.md`](./DEVELOPMENT.md): local development setup
- [`RELEASING.md`](./RELEASING.md): release process

---

## Relationship to agentregistry

This repository is a downstream, open-source fork of [`agentregistry`](https://github.com/agentregistry-dev/agentregistry). It keeps the registry, CLI, and web UI foundations while focusing the product experience on self-hosted Skill Hub usage, SHUB package distribution, private deployment, API-key authentication, and fallback source governance.

---

## Community

- Discord: https://discord.gg/HTYNjF2y2t
- Issues: https://github.com/yaogdu/skill-hub/issues
- Releases: https://github.com/yaogdu/skill-hub/releases

See [`CONTRIBUTING.md`](./CONTRIBUTING.md), [`DEVELOPMENT.md`](./DEVELOPMENT.md), and [`ARCHITECTURE.md`](./ARCHITECTURE.md) for contributor-facing docs.

---

## License

Apache 2.0, see [`LICENSE`](./LICENSE).
