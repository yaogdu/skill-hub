package validators_test

import (
	"os"
	"strings"
	"testing"
)

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
