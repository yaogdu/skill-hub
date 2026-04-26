---
name: native-skill
description: Example skill package for native Codex and Claude Code skill-directory exports.
version: 1.0.0
allowed-tools:
  - Read
  - Write
  - Edit
shub:
  schemaVersion: shub.skill/v1alpha1
  id: examples/native-skill
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
    - target: claude-code
      mode: skill-dir
      source: .
---
# Native Skill Example

This example is meant to be copied as a native skill directory into Codex and Claude Code.

See `docs/checklist.md` for a small companion file that should travel with the exported directory.
