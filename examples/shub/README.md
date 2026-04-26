# SHUB Example Assets

This directory contains ready-to-package `SKILL.md`-rooted assets that exercise the main SHUB export modes end to end.

## Included Examples

- `prompt-multi-target/`: a prompt asset that exports to Codex prompt files, Claude Code commands, Cursor rules, and Aider conventions.
- `native-skill/`: a prompt asset that exports native skill directories for both Codex and Claude Code.
- `mcp-remote-weather/`: an MCP asset rooted by `server.json` that exports a Claude Code `mcp-config` snippet for managed `.mcp.json` integration.

## Validate And Package

```bash
arctl shub lint examples/shub/prompt-multi-target
arctl shub package examples/shub/prompt-multi-target

arctl shub lint examples/shub/native-skill
arctl shub package examples/shub/native-skill

arctl shub lint examples/shub/mcp-remote-weather
arctl shub package examples/shub/mcp-remote-weather
```

## Local Workspace Export Demo

```bash
export SHUB_CLAUDE_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"
export SHUB_CURSOR_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"
export SHUB_AIDER_WORKSPACE_DIR="$PWD/.tmp/shub-workspace"

# after publishing to a registry and installing with npx shub add ...
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
