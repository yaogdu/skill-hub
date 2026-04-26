---
name: prompt-multi-target
description: Example prompt package that exports to multiple third-party tools.
version: 1.0.0
allowed-tools:
  - Read
  - Grep
shub:
  schemaVersion: shub.skill/v1alpha1
  id: examples/prompt-multi-target
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
    - target: cursor
      mode: rules-file
      source: SKILL.md
    - target: aider
      mode: rules-file
      source: SKILL.md
---
# Prompt Multi-Target

Use this package when you need a single instruction source to fan out into Codex, Claude Code, Cursor, and Aider.

## Behavior

- Read the local repository before changing code.
- Prefer minimal diffs that preserve the current structure.
- Summarize the user-visible outcome and the verification you ran.
