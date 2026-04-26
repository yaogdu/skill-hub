package shubsource

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/agentregistry-dev/agentregistry/internal/cli/common/gitutil"
	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

type Fetcher interface {
	Fetch(ctx context.Context, source *models.SHUBSource, assetID, version, targetDir string) (string, error)
}

type gitFetcher struct{}

func (gitFetcher) Fetch(_ context.Context, source *models.SHUBSource, assetID, version, targetDir string) (string, error) {
	resolved, err := resolveSourceAddress(source, assetID, version)
	if err != nil {
		return "", err
	}
	if err := gitutil.CloneAndCopy(resolved, targetDir, false); err != nil {
		return "", fmt.Errorf("clone SHUB source %q: %w", resolved, err)
	}
	return resolved, nil
}

func resolveSourceAddress(source *models.SHUBSource, assetID, version string) (string, error) {
	if source == nil {
		return "", fmt.Errorf("SHUB source is required")
	}
	address := strings.TrimSpace(source.Address)
	if address == "" {
		return "", fmt.Errorf("SHUB source %q address is empty", source.Name)
	}
	assetName := assetBaseName(assetID)
	if assetName == "" {
		return "", fmt.Errorf("asset id is required")
	}
	if strings.Contains(address, "{version}") && strings.TrimSpace(version) == "" {
		return "", fmt.Errorf("SHUB source %q requires a version but none was requested", source.Name)
	}
	if strings.Contains(address, "{asset}") || strings.Contains(address, "{name}") || strings.Contains(address, "{version}") {
		replacer := strings.NewReplacer(
			"{asset}", strings.Trim(strings.TrimSpace(assetID), "/"),
			"{name}", assetName,
			"{version}", strings.TrimSpace(version),
		)
		return replacer.Replace(address), nil
	}
	return strings.TrimRight(address, "/") + "/" + assetName, nil
}

func assetBaseName(assetID string) string {
	trimmed := strings.Trim(strings.TrimSpace(assetID), "/")
	if trimmed == "" {
		return ""
	}
	base := path.Base(trimmed)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
