package registries_test

import (
	"context"
	"testing"

	"github.com/agentregistry-dev/agentregistry/internal/registry/validators/registries"
	"github.com/modelcontextprotocol/registry/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNuGet_RealPackages(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		packageName  string
		version      string
		serverName   string
		expectError  bool
		errorMessage string
		networkBound bool
	}{
		{
			name:         "empty package identifier should fail",
			packageName:  "",
			version:      "1.0.0",
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "package identifier is required for NuGet packages",
		},
		{
			name:         "empty package version should fail",
			packageName:  "test-package",
			version:      "",
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "package version is required for NuGet packages",
		},
		{
			name:         "both empty identifier and version should fail with identifier error first",
			packageName:  "",
			version:      "",
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "package identifier is required for NuGet packages",
		},
		{
			name:         "non-existent package should fail",
			packageName:  generateRandomNuGetPackageName(),
			version:      "1.0.0",
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "ownership validation failed",
			networkBound: true,
		},
		{
			name:         "real package without version should fail",
			packageName:  "Newtonsoft.Json",
			version:      "", // No version provided
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "package version is required for NuGet packages",
		},
		{
			name:         "real package with non-existent version should fail",
			packageName:  "Newtonsoft.Json",
			version:      "999.999.999", // Version that doesn't exist
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "ownership validation failed",
			networkBound: true,
		},
		{
			name:         "real package without server name in README should fail",
			packageName:  "Newtonsoft.Json",
			version:      "13.0.3", // Popular version
			serverName:   "com.example/test",
			expectError:  true,
			errorMessage: "ownership validation failed",
			networkBound: true,
		},
		{
			name:         "real mcp package with different server name should fail",
			packageName:  "TimeMcpServer",
			version:      "1.0.2",
			serverName:   "io.github.domdomegg/not-time-mcp-server",
			expectError:  true,
			errorMessage: "ownership validation failed",
			networkBound: true,
		},
		{
			name:         "real package with server name in README should pass",
			packageName:  "NuGet.Mcp.Server",
			version:      "1.3.2",
			serverName:   "com.microsoft/nuget",
			expectError:  false,
			networkBound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.networkBound {
				skipUnlessLiveRegistryTests(t)
			}

			pkg := model.Package{
				RegistryType: model.RegistryTypeNuGet,
				Identifier:   tt.packageName,
				Version:      tt.version,
			}

			err := registries.ValidateNuGet(ctx, pkg, tt.serverName)

			if tt.expectError {
				require.Error(t, err)
				if tt.networkBound && isNetworkError(err.Error()) {
					t.Skipf("skipping due to transient network error: %v", err)
				}
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				if err != nil && tt.networkBound && isNetworkError(err.Error()) {
					t.Skipf("skipping due to transient network error: %v", err)
				}
				require.NoError(t, err)
			}
		})
	}
}
