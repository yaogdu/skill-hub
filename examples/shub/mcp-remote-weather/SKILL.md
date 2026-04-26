---
name: mcp-remote-weather
description: Example MCP package that exposes a remote weather server to Claude Code.
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: examples/mcp-remote-weather
  category: mcp
  entry:
    kind: mcp-config
    path: server.json
  runtime:
    type: remote
  exports:
    - target: claude-code
      mode: mcp-config
      source: server.json
---
# Remote Weather MCP

This example demonstrates a SHUB-managed MCP asset whose packaged `server.json` can be merged into Claude Code's project `.mcp.json`.
