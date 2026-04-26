package skill

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentregistry-dev/agentregistry/internal/cli/skill/templates"
	shubskills "github.com/agentregistry-dev/agentregistry/pkg/skills"
	"github.com/spf13/cobra"
)

func saveLintFlags(t *testing.T) {
	t.Helper()
	origOutputFormat := lintOutputFormat
	t.Cleanup(func() {
		lintOutputFormat = origOutputFormat
	})
}

func TestRunLint_ValidJSON(t *testing.T) {
	saveLintFlags(t)
	lintOutputFormat = "json"

	dir := t.TempDir()
	writeLintFile(t, filepath.Join(dir, "SKILL.md"), `---
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
`)

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := runLint(cmd, []string{dir}); err != nil {
		t.Fatalf("runLint() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"schemaVersion": "shub.asset/v1alpha1"`) {
		t.Fatalf("output = %q, want schemaVersion JSON", output)
	}
	if !strings.Contains(output, `"id": "local/helper-skill"`) {
		t.Fatalf("output = %q, want asset id JSON", output)
	}
}

func TestRunLint_InvalidSkill(t *testing.T) {
	saveLintFlags(t)
	lintOutputFormat = "text"

	dir := t.TempDir()
	writeLintFile(t, filepath.Join(dir, "SKILL.md"), `---
name: broken
description: Broken skill
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/broken
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: python
---
# Broken
`)

	err := runLint(&cobra.Command{}, []string{dir})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "skill validation failed") {
		t.Fatalf("error = %q, want validation prefix", err.Error())
	}
}

func TestRunLint_UnsupportedOutput(t *testing.T) {
	saveLintFlags(t)
	lintOutputFormat = "yaml"

	dir := t.TempDir()
	writeLintFile(t, filepath.Join(dir, "SKILL.md"), `---
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
`)

	err := runLint(&cobra.Command{}, []string{dir})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("error = %q, want unsupported output error", err.Error())
	}
}

func TestGeneratedEmptySkillPassesValidation(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "demo-skill")
	err := templates.NewGenerator().GenerateProject(templates.ProjectConfig{
		NoGit:       true,
		Directory:   projectDir,
		ProjectName: "demo-skill",
		Empty:       true,
	})
	if err != nil {
		t.Fatalf("GenerateProject() error = %v", err)
	}

	result, err := shubskills.ValidateDir(projectDir)
	if err != nil {
		t.Fatalf("ValidateDir() error = %v", err)
	}
	if result.Asset.ID != "local/demo-skill" {
		t.Fatalf("Asset.ID = %q, want %q", result.Asset.ID, "local/demo-skill")
	}
}

func writeLintFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create parent directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}
