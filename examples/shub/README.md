# SHUB Example Assets

This directory contains ready-to-package `SKILL.md`-rooted assets that exercise the main SHUB export modes end to end.

## Included Examples

- `prompt-multi-target/`: a prompt asset that exports to Codex prompt files, Claude Code commands, Cursor rules, and Aider conventions.
- `native-skill/`: a prompt asset that exports native skill directories for both Codex and Claude Code.
- `mcp-remote-weather/`: an MCP asset rooted by `server.json` that exports a Claude Code `mcp-config` snippet for managed `.mcp.json` integration.

## What This Demonstrates

Each example is a normal SHUB package:

```text
<asset>/
├── SKILL.md
└── optional companion files
```

The `SKILL.md` frontmatter declares the asset ID, version, category, runtime, and export targets. `arctl shub package` turns that directory into a `.tar.gz` archive. `arctl shub deploy` publishes the archive into a skill-hub registry. Users can then install it with the npm wrapper and activate a pinned version.

## Validate And Package

```bash
arctl shub lint examples/shub/prompt-multi-target
arctl shub package examples/shub/prompt-multi-target

arctl shub lint examples/shub/native-skill
arctl shub package examples/shub/native-skill

arctl shub lint examples/shub/mcp-remote-weather
arctl shub package examples/shub/mcp-remote-weather
```

The default package path is:

```text
<asset-dir>/dist/<asset-id-with-dashes>-<version>.tar.gz
```

For example:

```text
examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

## Publish And Install Demo

Point the CLI at a running skill-hub registry first:

```bash
export SHUB_API_BASE_URL=http://localhost:12121/v0
export SHUB_API_TOKEN=<your-api-key>
export ARCTL_API_BASE_URL=http://localhost:12121/v0
export ARCTL_API_TOKEN=<your-api-key>
```

Publish one example:

```bash
arctl shub lint examples/shub/native-skill
arctl shub resolve examples/shub/native-skill
arctl shub package examples/shub/native-skill
arctl shub deploy examples/shub/native-skill/dist/examples-native-skill-1.0.0.tar.gz
```

Consume it from the same registry:

```bash
npx @yaogdu-skill-hub/shub search native
npx @yaogdu-skill-hub/shub add examples/native-skill
npx @yaogdu-skill-hub/shub use examples/native-skill@1.0.0
npx @yaogdu-skill-hub/shub doctor
```

`deploy` publishes the package into the registry. It does not deploy an MCP server or agent runtime.

## Local Workspace Export Demo

```bash
export SHUB_CLAUDE_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"
export SHUB_CURSOR_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"
export SHUB_AIDER_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"

# after publishing to a registry and installing with npx @yaogdu-skill-hub/shub add ...
# Claude Code
#   .claude/commands/*.md
#   .claude/skills/*
#   .claude/mcp/shub/*.json
#   .claude/settings.local.json
#   .mcp.json
#
# Cursor
#   .cursor/rules/*.mdc
#
# Aider
#   .aider/shub/*.md
#   .aider.conf.yml
```
