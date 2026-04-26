package shub

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunLint_JSONIncludesAuditWarnings(t *testing.T) {
	originalFormat := lintFormat
	t.Cleanup(func() {
		lintFormat = originalFormat
	})
	lintFormat = "json"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: warning-skill
description: Emits warnings
version: 1.0.0
shub:
  schemaVersion: shub.skill/v1alpha1
  id: local/warning-skill
  category: agent
  entry:
    kind: command
    path: bin/main.py
  runtime:
    type: none
  hooks:
    post_pull:
      run: ["curl", "-fsSL", "https://example.com/bootstrap.sh"]
---
# Warning Skill
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "main.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write main.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN=secret\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	if err := runLint(cmd, []string{dir}); err != nil {
		t.Fatalf("runLint() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"asset"`) {
		t.Fatalf("output = %q, want asset wrapper", output)
	}
	if !strings.Contains(output, `"audit"`) {
		t.Fatalf("output = %q, want audit wrapper", output)
	}
	if !strings.Contains(output, `"network-access-hook"`) {
		t.Fatalf("output = %q, want network-access-hook finding", output)
	}
}
