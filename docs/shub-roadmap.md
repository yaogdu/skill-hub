# SHUB Roadmap

This roadmap summarizes the intended evolution of skill-hub as an enterprise AI asset registry and distribution layer.

## Product Positioning

SHUB should be treated as an internal AI asset supply chain system, similar in spirit to Nexus or Artifactory, but for AI capabilities instead of Maven jars or OCI images.

The assets managed by SHUB include:

- prompts
- skills rooted by `SKILL.md`
- MCP servers
- agents
- derived package metadata
- tool-specific exports for Codex, Claude Code, Cursor, Aider, and similar clients

The core value is not to become a public marketplace or a full agent runtime platform. The core value is to help enterprises publish, discover, install, version, audit, reproduce, and distribute internal AI assets.

## Target Use Case

A self-developed enterprise agent should be able to consume SHUB-managed assets during CI/CD, build against fixed versions, and then publish itself back to SHUB as a new versioned asset.

Expected flow:

```text
agent source repository
  -> CI trigger
  -> resolve prompts / skills / MCP assets from SHUB
  -> materialize fixed dependency versions
  -> run tests, evals, audits, and package checks
  -> build the agent package
  -> publish agent@version back to SHUB
  -> optionally promote release metadata to staging / production
```

This makes an agent release reproducible and auditable. A production agent should record both its own version and the exact SHUB asset versions it was built with.

## Non-Goals

SHUB should avoid expanding into these areas as first-class product scope:

- a public open marketplace
- a complete low-code workflow builder
- a full agent orchestration framework
- a full prompt observability platform
- a complete model registry
- a runtime deployment platform for MCP servers or agents
- a replacement for Langfuse, Promptfoo, MLflow, Dify, LangGraph, or MCP Registry

SHUB can integrate with those systems, but its strongest product boundary is asset registry, dependency resolution, distribution, and governance.

## Roadmap Themes

### 1. Asset-First Product Convergence

Unify product language, APIs, CLI commands, and UI around the `Asset` model.

Current code still has legacy concepts such as `skill`, `prompt`, and `agent` as separate user-facing surfaces. These should become compatibility paths behind a primary asset model.

Planned work:

- Make `/v0/assets` the primary browse and publish API.
- Update UI navigation to show assets first, with category filters for `prompt`, `skill`, `mcp`, and `agent`.
- Rename CLI help and arguments from `skill-name` to `asset-id`.
- Keep legacy endpoints temporarily as migration and compatibility adapters.
- Align documentation, examples, npm wrapper language, and release artifacts around `shub`.

### 2. Dependency Manifest

Introduce a package-level dependency manifest so an agent or skill can declare the SHUB assets it needs.

Example:

```yaml
dependencies:
  prompts:
    - security/code-review@1.2.0
  mcps:
    - infra/k8s-readonly@0.8.3
  skills:
    - arch/java-analyzer@2.1.0
```

Planned work:

- Define dependency fields in the SHUB manifest schema. Initial support is documented in `docs/shub-skill-frontmatter.schema.json` and `docs/shub-asset-manifest.schema.json`.
- Support dependency declarations in `SKILL.md` under `shub.dependencies`.
- Validate dependency syntax during `shub lint`, including duplicate references and floating `latest` versions.
- Resolve dependencies during CI and local install through `shub resolve`.
- Reject production publishes that rely on floating `latest` versions unless explicitly allowed by policy.

### 3. Lockfile and Reproducible Builds

Add a lockfile that records exactly what was resolved for a build.

Example:

```json
{
  "lockfileVersion": 1,
  "asset": "payment-risk-agent",
  "version": "1.4.0",
  "resolvedAssets": [
    {
      "id": "security/audit-checklist",
      "version": "2.1.0",
      "digest": "sha256:...",
      "sourceCommit": "..."
    },
    {
      "id": "infra/k8s-readonly",
      "version": "0.8.0",
      "digest": "sha256:...",
      "sourceCommit": "..."
    }
  ]
}
```

Planned commands:

```bash
shub resolve
shub install --frozen-lockfile
shub update <asset-id>
```

Planned work:

- Add `shub.lock` schema. Initial schema lives at `docs/shub-lock.schema.json`.
- Resolve asset versions and package/source metadata through `shub resolve`.
- Support frozen CI checks that fail if the lockfile and manifest differ through `shub resolve --check`.
- Store lockfile metadata with published assets.
- Allow local offline installs from lockfile when packages are already cached.

### 4. Dependency Graph and Lineage

Record and expose which assets depend on which other assets.

Questions SHUB should answer:

- Which agents use this prompt?
- Which agents depend on this MCP server?
- If an MCP server version is vulnerable, what production assets are affected?
- What exact prompt and skill versions were used to build `agent@1.4.0`?
- Which assets are safe to deprecate?

Planned work:

- Persist dependency edges on publish.
- Add API endpoints for forward and reverse dependencies.
- Add UI views for dependency graph and usage impact.
- Show dependency history per asset version.
- Support deprecation warnings when installed or published assets reference deprecated versions.

### 5. CI/CD Quality Gates

Make SHUB a useful CI/CD gate for enterprise AI assets.

Target publish flow:

```bash
shub lint
shub resolve
shub test
shub audit
shub package
shub deploy  # compatibility name for publishing the SHUB package
```

Planned work:

- Add CI-friendly output formats for all publish commands.
- Integrate prompt and agent evals through tools such as Promptfoo.
- Audit allowed tools, scripts, MCP permissions, runtime declarations, and external network usage.
- Enforce digest checks before publish.
- Support policies such as "no latest in prod", "no unapproved MCP in prod", and "eval score must pass threshold".
- Record CI run metadata, source repo, commit, actor, and policy results in published asset metadata.

### 6. Environment Promotion

Add controlled promotion across environments.

Example:

```bash
shub promote payment-risk-agent@1.4.0 --to staging
shub promote payment-risk-agent@1.4.0 --to prod
```

Planned work:

- Add environment metadata such as `dev`, `staging`, and `prod`.
- Support per-environment policy requirements.
- Require stronger checks for production promotion.
- Preserve immutable package digests across environments.
- Expose current environment status in UI and API.

### 7. Trust, Signatures, and Attestations

Move SHUB from a package registry toward a trusted AI asset supply chain.

Planned work:

- Store package digest and source digest for every asset version.
- Record source repository, commit, tag, CI run ID, and publisher identity.
- Support package signing and signature verification.
- Add attestation metadata for lint, eval, audit, and dependency resolution results.
- Add SBOM-like asset composition metadata for agents.
- Block or warn on untrusted sources depending on policy.

### 8. Tool Export Adapters

Continue improving the compatibility layer that exports SHUB assets to developer tools.

Target command shape:

```bash
shub export --target codex
shub export --target claude-code
shub export --target cursor
shub export --target aider
```

Planned work:

- Keep flat prompt exports for simple and stable compatibility.
- Support native Codex skill directory exports.
- Improve Claude Code skill, command, and MCP config exports.
- Improve Cursor rules exports.
- Improve Aider rules exports.
- Make `shub doctor` repair all SHUB-owned integration files.
- Add dry-run output so users can inspect changed files before export.

### 9. Offline and Local State Reliability

Keep local use reliable after assets have been installed.

Planned work:

- Improve `~/.shub/state.json` migration support.
- Add local package integrity verification.
- Add cache pruning and repair commands.
- Support offline search from installed metadata.
- Ensure `shub use` can switch between cached versions without network access.
- Make concurrent installs and exports robust through locking and transactional state writes.

### 10. Enterprise Governance UI

The UI should stay read-first and governance-focused rather than becoming a broad authoring surface.

Planned work:

- Asset search and category filters.
- Version history and package metadata.
- Dependency graph and reverse dependency impact.
- Published package digests and source metadata.
- Policy status, eval status, and audit status.
- Deprecated, blocked, approved, and promoted states.
- API key and fallback source administration.

## Suggested Milestones

### P0: Product Convergence

- Asset-first UI and CLI language.
- Primary `/v0/assets` browse, publish, and package flows.
- `shub add <asset-id>` terminology and behavior cleanup.
- Documentation aligned around "AI asset registry similar to Nexus".

### P1: Dependency and Lockfile Foundation

- `shub.dependencies` schema.
- `shub resolve`.
- `shub.lock`.
- Registry target and token resolution reused from existing CLI configuration.
- `shub install --frozen-lockfile`.
- Store resolved dependency metadata during publish.

### P2: Dependency Graph and CI Gates

- Forward and reverse dependency APIs.
- UI dependency graph.
- CI-friendly lint, resolve, audit, and eval outputs.
- Basic policy enforcement for production publishes.

### P3: Promotion and Trust

- Environment promotion.
- Digest and signature enforcement.
- Attestations for CI checks.
- SBOM-like agent composition metadata.

### P4: Advanced Integrations

- Richer Codex, Claude Code, Cursor, and Aider exports.
- Deeper Promptfoo and Langfuse integrations.
- Compatibility with emerging agent discovery standards such as A2A Agent Cards where useful.

## Success Criteria

SHUB is successful when a team can:

- publish AI assets through CI/CD only
- resolve fixed versions during agent builds
- reproduce a production agent from its lockfile
- audit exactly which prompts, skills, and MCP servers a production agent uses
- detect impact when an asset version is deprecated or vulnerable
- install and use approved assets locally through `shub`
- keep installed assets usable offline
- export assets into common AI development tools without manual configuration
