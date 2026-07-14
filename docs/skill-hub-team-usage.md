# Skill Hub Team Usage

这份文档说明团队怎样让 Codex 使用 174 上的 Skill Hub 测试环境。它不包含密码和 API token，可以放进仓库；带凭证的本地版见 `docs/skill-hub-team-usage.private.md`。

## 1. 先分清三件事

GitHub 是源代码和审计历史：

- 存放 `SKILL.md`、工程文件、文档、脚本和测试。
- 通过 commit、branch、PR、tag 记录变更。
- 需要长期追溯时，以 GitHub commit/tag 为准。

Skill Hub 是发布后的资产注册表：

- 存放已经打包发布的 skill、prompt、agent、MCP 等资产版本。
- 负责搜索、安装、切换版本、依赖解析和本地导出。
- 不替代 GitHub，也不是运行时部署平台。

本地私有文档和脚本是接入材料：

- 存放测试环境 URL、账号、API token 和 Codex 操作提示。
- 只在团队内部手动分发，不提交 GitHub。
- 让 Codex 在一台新电脑上也知道如何检查工具、配置环境、连接 Skill Hub。

简化理解：

```text
GitHub = 源文件和历史
Skill Hub = 发布后的版本化资产仓库
私有文档/脚本 = 测试环境接入凭证和操作入口
```

## 2. 当前测试环境

Web UI:

```text
http://192.144.187.174:32121
```

API:

```text
http://192.144.187.174:32121/v0
```

用途：

- 验证 Skill Hub 的资产发布、搜索、安装、版本切换和依赖解析。
- 给团队试用 Codex + Skill Hub 的工作流。

不要用于：

- 生产服务。
- agent / MCP 的运行时部署。
- 修改 174 上其他服务。

## 3. 网络边界

174 的 `32121` 端口按来源公网 IP 放行。如果无法访问，先查当前出口 IP：

```bash
curl ifconfig.me
```

把返回的 IP 发给管理员，只放行 TCP `32121`。不要为了测试直接要求开放到全网。

## 4. Codex 从零环境启动

不要假设本机已经有 `shub`。Codex 应按这个顺序找工具：

1. 如果 shell 里已有 `skillhub_shub`，直接用它。
2. 如果系统有 `arctl`，使用 `arctl shub ...`。
3. 如果当前仓库有 `./bin/arctl`，使用 `./bin/arctl shub ...`。
4. 如果当前仓库是 skill-hub 源码仓库、有 Go 环境、但没有二进制，运行：

   ```bash
   make build-cli
   ```

   然后使用 `./bin/arctl shub ...`。

5. 如果没有 Go 环境，但有 Node.js 18+ / `npx`，使用：

   ```bash
   npx -y @yaogdu-skill-hub/shub <command>
   ```

   npm wrapper 会下载 GitHub Release 里的预编译 `arctl`，所以不需要本机 Go。

6. 如果既没有 `arctl`、也没有 Go、也没有 Node.js，再让用户选择安装工具或让管理员提供对应系统的预编译 `arctl`。

环境变量：

```bash
export SHUB_API_BASE_URL=http://192.144.187.174:32121/v0
export ARCTL_API_BASE_URL=http://192.144.187.174:32121/v0
export SHUB_API_TOKEN=<api-token>
export ARCTL_API_TOKEN=<api-token>
```

公开仓库文档不要写真实 token。

## 5. 本地私有脚本

私有脚本建议随私有文档一起发给团队：

```text
scripts/skill-hub-test-env.private.sh
```

使用方式：

```bash
source scripts/skill-hub-test-env.private.sh
skillhub_check
```

它会提供：

- `skillhub_arctl`: 自动选择系统 `arctl`、`./bin/arctl` 或提示构建。
- `skillhub_shub`: 调用 `arctl shub ...`；如果没有 `arctl` 但有 `npx`，会回退到 `npx -y @yaogdu-skill-hub/shub`。
- `skillhub_check`: 检查公网出口 IP、Skill Hub API 和 CLI。
- `skillhub_bootstrap_help`: 打印本地初始化步骤。
- `skillhub_codex_prompt`: 打印可直接贴给 Codex 的操作指令。
- `skillhub_publish_package <skill-dir>`: 对一个 skill 目录执行 lint、resolve、package、deploy。

## 6. 消费资产

如果用户没有提供版本，Codex 必须提醒：

```text
你没有指定版本。我可以先查可用版本；如果继续安装，会使用当前 latest。是否继续？
```

可以先搜索或列版本，但不要在没有提醒的情况下直接把未指定版本解释成固定版本。

搜索：

```bash
skillhub_shub search <keyword>
```

安装：

```bash
skillhub_shub add <asset-id>
```

切换版本：

```bash
skillhub_shub use <asset-id>@<version>
```

检查本地状态：

```bash
skillhub_shub doctor
```

Codex 完成后应报告：

- asset id
- 安装或激活的版本
- 本地安装/export 路径
- `doctor` 结果

## 7. 发布资产

发布对象应该是一个以 `SKILL.md` 为根的目录，且 frontmatter 至少包含：

```yaml
---
name: example-skill
description: What this skill does and when to use it.
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: team/example-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
  exports:
    - target: codex
      mode: skill-dir
      source: .
---
```

发布规则：

- `version` 必须明确写在 `SKILL.md` frontmatter 中。
- 如果用户要求“发布/同步”但没有说明版本，Codex 必须先读 `SKILL.md` 的 `version` 并提醒用户将要发布的精确版本。
- 如果 `SKILL.md` 没有 `version`，停止并让用户确认版本号。
- 同一个 `shub.id@version` 默认不可覆盖；版本发布后应视为不可变。
- 测试环境也不要静默覆盖。若用户明确要求覆盖，必须让用户确认精确的 `asset-id@version` 和处理方式。优先建议发布新版本，例如 `1.0.1` 或 `1.0.0-test.2`。

发布命令：

```bash
skillhub_shub lint ./your-skill
skillhub_shub resolve ./your-skill
skillhub_shub package ./your-skill
skillhub_shub deploy ./your-skill/dist/*.tar.gz
```

如果没有使用私有脚本，可以等价使用：

```bash
./bin/arctl shub lint ./your-skill
./bin/arctl shub resolve ./your-skill
./bin/arctl shub package ./your-skill
./bin/arctl shub deploy ./your-skill/dist/*.tar.gz
```

发布后验证：

```bash
skillhub_shub search <asset-keyword>
skillhub_shub add <asset-id>
skillhub_shub use <asset-id>@<version>
skillhub_shub doctor
```

## 8. GitHub 与发布的关系

推荐正式流程：

```text
修改源文件
-> 本地测试
-> commit / PR / merge
-> tag
-> package
-> publish to Skill Hub
-> 下游使用 Skill Hub 中的固定版本
```

测试阶段可以从本地目录直接发布到 Skill Hub，但要明确这是 local-only provenance：

- 可以验证 Skill Hub 功能。
- 不适合作为长期可审计版本。
- 后续应补 GitHub commit/tag 或重新发一个有来源的版本。

Codex 不应自行执行这些动作，除非用户明确要求：

- 修改版本号。
- `git commit`
- `git tag`
- `git push`
- 发布到 Skill Hub。
- 修改 GitHub 仓库设置。
- 删除旧版本或覆盖同一版本。

## 9. 给 Codex 的提示词

把下面这段贴给 Codex，再附上私有文档或让它先 source 私有脚本：

```text
You are operating the team Skill Hub test environment.

Assume the local machine may not have shub or arctl installed. Do not guess.

First inspect the current directory, Git state, SKILL.md files, README files,
Makefile, ./bin/arctl, package scripts, and shub.lock.

Resolve the CLI in this order:
1. skillhub_shub
2. arctl shub
3. ./bin/arctl shub
4. If Go is available in the skill-hub repository, run make build-cli, then use ./bin/arctl shub.
5. If Node.js 18+ / npx is available, use npx -y @yaogdu-skill-hub/shub.
6. Ask before installing other tools or downloading binaries.
7. If npx installs an older arctl and package download fails with 401, ask for a newer GitHub release or a provided arctl binary.

Use the configured environment:
- SHUB_API_BASE_URL
- ARCTL_API_BASE_URL
- SHUB_API_TOKEN
- ARCTL_API_TOKEN

For consuming assets, run search/add/use/doctor and report asset id, version,
paths, and verification result. If the user did not specify a version, remind
them that the command will use latest or first list available versions.

For publishing assets, publish only valid SKILL.md-rooted packages. Run lint,
resolve, package, deploy, then verify by search or add/use. Do not change
versions, create Git commits/tags, push to GitHub, or publish unless explicitly
asked. If the user did not specify a version, read SKILL.md and confirm the
exact version before publishing. Do not overwrite an existing asset@version by
default; require explicit confirmation for the exact asset id and version, and
prefer publishing a new version.

Boundaries:
- GitHub is source history.
- Skill Hub is the versioned asset registry.
- The test instance is not a runtime deployment platform.
- Do not modify cloud security groups, DNS, Traefik, production services, or
  GitHub settings unless explicitly asked.
- Do not commit passwords, API tokens, or private env files.
```

## 10. 排障

页面打不开：

```bash
curl ifconfig.me
curl -I http://192.144.187.174:32121
```

API 可达但命令失败：

```bash
echo "$SHUB_API_BASE_URL"
echo "$ARCTL_API_BASE_URL"
skillhub_shub search test
```

CLI 不存在：

```bash
make build-cli
./bin/arctl shub --help
```

认证失败：

```bash
echo "$SHUB_API_TOKEN"
echo "$ARCTL_API_TOKEN"
```

确认 token 存在；如果 token 泄露或不可用，在 Skill Hub 后台重新创建 API key。
