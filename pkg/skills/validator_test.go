package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAssetDir_ValidPromptSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: helper-skill
description: Helpful prompt skill
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/helper-skill
  category: prompt
  entry:
    kind: skill-body
    path: SKILL.md
  runtime:
    type: none
---
# Helper Skill

Use this skill for guidance.
`)

	asset, err := LoadAssetDir(dir)
	if err != nil {
		t.Fatalf("LoadAssetDir() error = %v", err)
	}
	if asset.ID != "local/helper-skill" {
		t.Fatalf("ID = %q, want %q", asset.ID, "local/helper-skill")
	}
	if asset.Manifest.Entry.Path != "SKILL.md" {
		t.Fatalf("Entry.Path = %q, want SKILL.md", asset.Manifest.Entry.Path)
	}
}

func TestLoadAssetDir_ValidAgentSkill(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "bin"))
	writeFile(t, filepath.Join(dir, "bin", "main.py"), "print('ok')\n")
	writeFile(t, filepath.Join(dir, "pyproject.toml"), "[project]\nname='demo'\n")
	writeFile(t, filepath.Join(dir, "uv.lock"), "version = 1\n")
	writeSkillFile(t, dir, `---
name: java-analyzer
description: Analyze Java services
version: 1.2.0
allowed-tools:
  - Read
  - Bash
shub:
  schemaVersion: shub.skill/v1alpha1
  id: arch/java-analyzer
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: python
    version: ">=3.10"
    install:
      strategy: uv
      path: pyproject.toml
      lockfile: uv.lock
  dependencies:
    prompts:
      - security/code-review@1.2.0
    mcps:
      - id: infra/k8s-readonly
        version: 0.8.3
  exports:
    - target: codex
      mode: prompt-file
      source: SKILL.md
---
# Java Analyzer
`)

	asset, err := LoadAssetDir(dir)
	if err != nil {
		t.Fatalf("LoadAssetDir() error = %v", err)
	}
	if asset.Manifest.Runtime.Install == nil || asset.Manifest.Runtime.Install.Path != "pyproject.toml" {
		t.Fatalf("runtime install path not preserved: %+v", asset.Manifest.Runtime.Install)
	}
	if got := asset.Manifest.Dependencies.Prompts[0]; got.ID != "security/code-review" || got.Version != "1.2.0" {
		t.Fatalf("prompt dependency = %+v, want security/code-review@1.2.0", got)
	}
	if got := asset.Manifest.Dependencies.MCPs[0]; got.ID != "infra/k8s-readonly" || got.Version != "0.8.3" {
		t.Fatalf("mcp dependency = %+v, want infra/k8s-readonly@0.8.3", got)
	}
}

func TestLoadAssetDir_InvalidPackageReferences(t *testing.T) {
	tests := []struct {
		name        string
		skill       string
		files       map[string]string
		errContains string
	}{
		{
			name: "missing entry file",
			skill: `---
name: bad-agent
description: Missing entry
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/bad-agent
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: python
---
# Bad Agent
`,
			errContains: "entry.path does not exist",
		},
		{
			name: "path escapes package",
			skill: `---
name: bad-export
description: Export escapes
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/bad-export
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: none
  exports:
    - target: codex
      mode: prompt-file
      source: ../outside.md
---
# Bad Export
`,
			files:       map[string]string{"bin/main.py": "print('ok')\n"},
			errContains: "must stay within the skill package",
		},
		{
			name: "missing shub metadata",
			skill: `---
name: legacy
description: Legacy only
version: 1.0.0
---
# Legacy
`,
			errContains: "missing required field: shub.schemaVersion",
		},
		{
			name: "duplicate dependency reference",
			skill: `---
name: bad-deps
description: Duplicate deps
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/bad-deps
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: none
  dependencies:
    prompts:
      - security/code-review@1.2.0
    skills:
      - security/code-review@1.2.0
---
# Bad Deps
`,
			files:       map[string]string{"bin/main.py": "print('ok')\n"},
			errContains: "duplicate dependency reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for path, content := range tt.files {
				fullPath := filepath.Join(dir, path)
				mustMkdirAll(t, filepath.Dir(fullPath))
				writeFile(t, fullPath, content)
			}
			writeSkillFile(t, dir, tt.skill)

			_, err := LoadAssetDir(dir)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.errContains)
			}
		})
	}
}

func writeSkillFile(t *testing.T, dir, content string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "SKILL.md"), content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}
