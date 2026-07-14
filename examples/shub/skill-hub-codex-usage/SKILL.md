---
name: skill-hub-codex-usage
description: Operate the team Skill Hub test environment from Codex, including zero-tool bootstrap, registry authentication checks, asset search/install/use/doctor flows, SHUB package publish flows, version confirmation, and non-overwrite safety rules. Use when a user asks Codex to use Skill Hub, publish or sync a skill to Skill Hub, install a Skill Hub asset locally, or explain the GitHub plus Skill Hub workflow.
version: 1.0.1
allowed-tools:
  - Read
  - Grep
  - Bash
shub:
  schemaVersion: shub.skill/v1alpha1
  id: team/skill-hub-codex-usage
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
    - target: claude-code
      mode: prompt-file
      source: SKILL.md
---
# Skill Hub Codex Usage

Use this skill to operate the team Skill Hub test environment from Codex without assuming the local machine already has `shub` installed.

Do not include passwords or API tokens in published Skill Hub assets. Credentials must come from local environment variables, a local private handoff document, or a user-provided secret for the current session.

## Operating Model

- Treat GitHub as source history: files, commits, PRs, tags, review, and provenance.
- Treat Skill Hub as the versioned asset registry: published skill/prompt/agent/MCP packages, version lookup, install, use, dependency resolution, and local exports.
- Treat local private docs/scripts as access material: URL, admin login, API token, bootstrap helpers, and team-specific instructions.
- Treat the test Skill Hub instance as asset-management infrastructure, not a runtime deployment platform.

## 0. Inspect First

Before running registry actions, inspect the local context:

```bash
pwd
git status --short
find . -name SKILL.md
```

Look for `README.md`, `Makefile`, `./bin/arctl`, package scripts, `shub.lock`, and local private files such as `scripts/skill-hub-test-env.private.sh`.

Do not modify cloud security groups, DNS, Traefik, production services, GitHub settings, commits, tags, or version numbers unless the user explicitly asks.

## 1. Resolve The CLI

Do not assume `shub` exists. Pick the first available option:

1. If the shell has `skillhub_shub`, use `skillhub_shub`.
2. Else if `arctl` exists, use `arctl shub`.
3. Else if `./bin/arctl` exists, use `./bin/arctl shub`.
4. Else if this is the skill-hub repository and Go is available, run `make build-cli`, then use `./bin/arctl shub`.
5. Else if Node.js 18+ / `npx` is available, use `npx -y @yaogdu-skill-hub/shub`.
6. Else ask before installing external tools or getting a prebuilt `arctl` binary.

The npm wrapper does not require Go. It downloads a prebuilt `arctl` from the skill-hub GitHub release. If package install fails with a 401 while downloading a registry-hosted package, the release may be too old; ask for a newer release or a supplied `arctl` binary.

Use `SHUB_API_BASE_URL` / `ARCTL_API_BASE_URL` and `SHUB_API_TOKEN` / `ARCTL_API_TOKEN` from the environment. If credentials are missing, ask for them or ask the user to source the private bootstrap script.

## 2. Check Network And Auth

Check reachability before publishing or installing:

```bash
curl -I "$SKILL_HUB_URL"
```

If the hub is unreachable, get the current public egress IP:

```bash
curl ifconfig.me
```

Report that IP so it can be allowlisted for TCP `32121`. Do not change firewall or cloud security group rules unless explicitly asked.

## 3. Version Rules

If the user did not specify a version, remind them.

For consuming assets:

- Search/list first when possible.
- Say that `add <asset-id>` or `use <asset-id>` without a version may resolve to current `latest`.
- Prefer asking which version to pin when the task affects a shared workflow or reproducibility.

For publishing assets:

- Read `SKILL.md` and identify `shub.id` and `version`.
- If `version` is missing, stop and ask for the version.
- If the user asked to publish but did not mention a version, confirm the exact `shub.id@version` before deploying.
- Do not change `version` unless explicitly asked.

Do not overwrite the same `asset-id@version` by default. Published versions should be treated as immutable. If a duplicate publish fails, do not delete/recreate or route around it automatically. Ask whether to publish a new version, such as `1.0.1` or `1.0.0-test.2`. Only attempt replacement when the user explicitly confirms the exact `asset-id@version` and understands it weakens traceability in the test registry.

## 4. Consume Assets

Search:

```bash
<shub> search <keyword>
```

Install:

```bash
<shub> add <asset-id>
```

Activate a pinned version:

```bash
<shub> use <asset-id>@<version>
```

Verify local state:

```bash
<shub> doctor
```

Report the asset id, selected version, install/export paths, and verification result.

## 5. Publish Assets

Publish only a directory rooted by `SKILL.md` with valid SHUB frontmatter.

Run:

```bash
<shub> lint ./your-skill
<shub> resolve ./your-skill
<shub> package ./your-skill
<shub> deploy ./your-skill/dist/*.tar.gz
```

After deploy, verify:

```bash
<shub> search <asset-keyword>
```

If the version has no GitHub commit or tag, report that the provenance is local-only and suitable for testing rather than long-term audit.

## 6. Report Back

Summarize:

- CLI path used.
- Registry URL used.
- Asset id and version.
- Whether the operation used `latest` or a pinned version.
- Whether provenance is Git-backed or local-only.
- Verification commands and results.

Do not print tokens or passwords in the final response.
