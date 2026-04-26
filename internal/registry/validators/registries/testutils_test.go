package registries_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

func generateRandomPackageName() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a static name if crypto/rand fails
		return "nonexistent-pkg-fallback"
	}
	return fmt.Sprintf("nonexistent-pkg-%s", hex.EncodeToString(bytes))
}

func generateRandomNuGetPackageName() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "NonExistent.Package.Fallback"
	}
	return fmt.Sprintf("NonExistent.Package.%s", hex.EncodeToString(bytes)[:16])
}

func skipUnlessLiveRegistryTests(t *testing.T) {
	t.Helper()
	if !liveRegistryTestsEnabled() {
		t.Skip("skipping live registry validation test; set AGENT_REGISTRY_LIVE_REGISTRY_TESTS=1 to enable")
	}
}

func liveRegistryTestsEnabled() bool {
	value := strings.TrimSpace(os.Getenv("AGENT_REGISTRY_LIVE_REGISTRY_TESTS"))
	return value == "1" || strings.EqualFold(value, "true")
}

// isNetworkError returns true if the error message indicates a transient
// network issue (timeout, DNS failure, connection refused, etc.).
func isNetworkError(msg string) bool {
	patterns := []string{
		"context deadline exceeded",
		"connection refused",
		"no such host",
		"i/o timeout",
		"TLS handshake timeout",
		"Client.Timeout exceeded while awaiting headers",
		"timed out after 30 seconds",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
