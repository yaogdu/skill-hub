# Releasing skill-hub

This repository ships two related but distinct release surfaces:

- the `arctl` GitHub release artifacts, which power the main CLI and the binary downloaded by the npm wrapper
- the npm wrapper package `@yaogdu-skill-hub/shub`

In practice, the GitHub Release is the primary release. The npm wrapper should be republished when the wrapper code or npm package metadata changes, or when you want the npm package page to reflect updated documentation.

## What Must Be Released

Release the GitHub artifacts when:

- `cmd/cli`, `internal/cli`, `internal/client`, or registry/server behavior changed
- SHUB install, publish, auth, fallback-source, or export behavior changed
- you want `npx @yaogdu-skill-hub/shub` users to pick up a new `arctl` binary

Republish `@yaogdu-skill-hub/shub` when:

- `npm/shub/bin/shub.js` changed
- `npm/shub/package.json` changed
- `npm/shub/README.md` changed and you want the npm package page updated

## Recommended Order

1. Merge the release-ready commit to your default branch
2. Create and push a Git tag like `v0.1.1`
3. Let GitHub Actions build the multi-platform `arctl` binaries and GitHub Release
4. Let the same release workflow publish `@yaogdu-skill-hub/shub` if `NPM_TOKEN` is configured
5. Verify the published GitHub Release assets and npm package page

## Pre-Release Checklist

- README reflects the current branding, setup flow, and examples
- `npm/shub/package.json` version is bumped if npm needs republishing
- release notes-worthy changes are merged
- Docker image and local Compose flow still work
- targeted tests pass

Suggested checks:

```bash
GOCACHE=/tmp/agentregistry-go-cache go test ./internal/cli/shub -count=1
GOCACHE=/tmp/agentregistry-go-cache go test ./internal/registry/service/shubsource -count=1
```

## GitHub Release Workflow

This repository already includes `.github/workflows/release.yml`.

It triggers on tags matching `v*.*.*` and performs these steps:

- builds and pushes Docker images
- builds `arctl` release binaries and checksums via `make release-cli`
- packages and pushes the Helm chart
- creates a GitHub Release
- publishes `@yaogdu-skill-hub/shub` to npm if `NPM_TOKEN` is configured

## Manual Tagging

```bash
git tag v0.1.1
git push origin v0.1.1
```

If you need to build the CLI release artifacts locally before tagging:

```bash
make release-cli
```

## npm Wrapper Notes

The npm wrapper does not embed the full CLI. It resolves a local `arctl` binary when available and otherwise downloads the latest GitHub Release binary.

That means:

- many functional changes only require a new GitHub Release
- a new npm publish is still useful when the wrapper code, package metadata, or npm-facing docs change

## Required Secrets For Automated Publish

The GitHub release workflow expects:

- `NPM_TOKEN` for npm publish
- default GitHub token permissions for GitHub Release creation
- Helm registry credentials only if you override the defaults used in the workflow

If `NPM_TOKEN` is missing, the workflow skips npm publish and still completes the GitHub Release path.
