package config

import (
	"os"
	"strings"
	"testing"
)

func TestNewConfig_RuntimeDirHasRandomSuffix(t *testing.T) {
	// Ensure the env var is unset so the default path is used.
	os.Unsetenv("AGENT_REGISTRY_RUNTIME_DIR")

	cfg := NewConfig()

	base := "/tmp/arctl-runtime-"
	if !strings.HasPrefix(cfg.RuntimeDir, base) {
		t.Fatalf("RuntimeDir should start with %q, got %q", base, cfg.RuntimeDir)
	}

	suffix := strings.TrimPrefix(cfg.RuntimeDir, base)
	if len(suffix) != 16 { // 8 bytes = 16 hex chars
		t.Fatalf("RuntimeDir suffix should be 16 hex chars, got %q (len %d)", suffix, len(suffix))
	}
}

func TestNewConfig_RuntimeDirUniqueBetweenCalls(t *testing.T) {
	os.Unsetenv("AGENT_REGISTRY_RUNTIME_DIR")

	cfg1 := NewConfig()
	cfg2 := NewConfig()

	if cfg1.RuntimeDir == cfg2.RuntimeDir {
		t.Fatalf("two NewConfig() calls should produce different RuntimeDir values, both got %q", cfg1.RuntimeDir)
	}
}

func TestNewConfig_RuntimeDirRespectsEnvOverride(t *testing.T) {
	custom := "/custom/runtime/path"
	t.Setenv("AGENT_REGISTRY_RUNTIME_DIR", custom)

	cfg := NewConfig()

	if cfg.RuntimeDir != custom {
		t.Fatalf("RuntimeDir should be %q when env var is set, got %q", custom, cfg.RuntimeDir)
	}
}

func TestNewConfig_PublicActionsDefaultsEmpty(t *testing.T) {
	t.Setenv("AGENT_REGISTRY_PUBLIC_ACTIONS", "")

	cfg := NewConfig()

	if cfg.PublicActions != "" {
		t.Fatalf("PublicActions should default to empty string, got %q", cfg.PublicActions)
	}
}

func TestNewConfig_PublicActionsRespectsEnvOverride(t *testing.T) {
	t.Setenv("AGENT_REGISTRY_PUBLIC_ACTIONS", "read,publish")

	cfg := NewConfig()

	if cfg.PublicActions != "read,publish" {
		t.Fatalf("PublicActions should be %q when env var is set, got %q", "read,publish", cfg.PublicActions)
	}
}

func TestNewConfig_StorageDirRespectsEnvOverride(t *testing.T) {
	custom := "/custom/storage/path"
	t.Setenv("AGENT_REGISTRY_STORAGE_DIR", custom)

	cfg := NewConfig()

	if cfg.StorageDir != custom {
		t.Fatalf("StorageDir should be %q when env var is set, got %q", custom, cfg.StorageDir)
	}
}

func TestValidate_StorageDirRequired(t *testing.T) {
	cfg := NewConfig()
	cfg.StorageDir = "   "

	err := Validate(cfg)
	if err == nil {
		t.Fatal("Validate() error = nil, want storage dir validation error")
	}
	if !strings.Contains(err.Error(), "storage dir must not be empty") {
		t.Fatalf("Validate() error = %q, want storage dir validation error", err)
	}
}
