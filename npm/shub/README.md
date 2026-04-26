# @yaogdu-skill-hub/shub

`@yaogdu-skill-hub/shub` is the npm wrapper for the SHUB client shipped by `skill-hub`.

It resolves a local `arctl` binary when available and otherwise downloads the matching release artifact from GitHub, verifies its checksum, and forwards commands as:

```bash
arctl shub ...
```

## Usage

```bash
npx @yaogdu-skill-hub/shub search java
npx @yaogdu-skill-hub/shub add arch/java-analyzer
npx @yaogdu-skill-hub/shub add unfallenwill/supercoder -g
npx @yaogdu-skill-hub/shub add arch/java-analyzer --fallback-source github-main
npx @yaogdu-skill-hub/shub use arch/java-analyzer@1.2.0
```

## Connect To A Self-Hosted Registry

```bash
export SHUB_API_BASE_URL=http://localhost:12121/v0
export SHUB_API_TOKEN=<your-api-key>
```

```fish
set -gx SHUB_API_BASE_URL http://localhost:12121/v0
set -gx SHUB_API_TOKEN <your-api-key>
```

`@yaogdu-skill-hub/shub` reads `SHUB_API_BASE_URL` / `SHUB_API_TOKEN`. The underlying `arctl` binary also accepts `ARCTL_API_BASE_URL` / `ARCTL_API_TOKEN`.

## Fallback Behavior

- `shub add <asset-id>` first checks the registry
- On a miss, the client automatically tries the built-in fallback source pool and then any custom fallback sources configured by the registry administrator
- `-g` narrows that miss-handling flow to the GitHub-oriented built-in sources
- `--fallback-source <name>` forces an explicit source name and order

Built-in GitHub source patterns currently include:

- `github-direct` for repositories whose root is already a SHUB package
- `github-skills-main` for repositories that keep skills under `skills/<name>`
- `github-plugin-skills-main` for plugin repositories that keep skills under `plugins/<name>/skills/<name>`

If a fetched GitHub repository contains a plain `SKILL.md` but not full SHUB metadata yet, skill-hub will synthesize a minimal prompt asset and mirror it into the registry. When no explicit version is requested, the mirrored asset currently uses `0.0.0-imported`.

If you install it globally, the exposed binary name remains `shub`:

```bash
npm install -g @yaogdu-skill-hub/shub
shub search java
```

## Environment Overrides

- `SHUB_BINARY`: use an explicit `arctl` binary instead of downloading one
- `SHUB_RELEASE_TAG`: pin the GitHub release tag to use
- `SHUB_CACHE_DIR`: override the wrapper cache directory
- `SHUB_SKIP_CHECKSUM=1`: skip checksum verification
- `SHUB_CODEX_SKILLS_DIR`: override native Codex skill-dir exports
- `CODEX_HOME`: override the default `~/.codex` base directory

Project source: https://github.com/yaogdu/skill-hub
