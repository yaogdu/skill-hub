package shubsource

import (
	"sort"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

var builtinCatalog = []*models.SHUBSource{
	{
		Name:        "anthropic-skills",
		Address:     "https://github.com/anthropics/skills/tree/main/skills/{name}",
		Description: "Anthropic's official Claude Code skills catalog.",
		Provider:    "github",
		BuiltIn:     true,
	},
	{
		Name:        "github-direct",
		Address:     "https://github.com/{asset}",
		Description: "Treat the requested asset id as owner/repo and clone the GitHub repository root directly.",
		Provider:    "github",
		BuiltIn:     true,
	},
	{
		Name:        "github-skills-main",
		Address:     "https://github.com/{asset}/tree/main/skills/{name}",
		Description: "Treat the requested asset id as owner/repo, then look for skills/{name} on the repository's main branch.",
		Provider:    "github",
		BuiltIn:     true,
	},
	{
		Name:        "github-plugin-skills-main",
		Address:     "https://github.com/{asset}/tree/main/plugins/{name}/skills/{name}",
		Description: "Treat the requested asset id as owner/repo, then look for plugins/{name}/skills/{name} on the repository's main branch.",
		Provider:    "github",
		BuiltIn:     true,
	},
	{
		Name:        "openai-skills",
		Address:     "https://github.com/openai/skills/tree/main/skills/{name}",
		Description: "OpenAI's official Codex skills catalog.",
		Provider:    "github",
		BuiltIn:     true,
	},
}

func listBuiltInSources() []*models.SHUBSource {
	result := make([]*models.SHUBSource, 0, len(builtinCatalog))
	for _, source := range builtinCatalog {
		result = append(result, cloneSourceRecord(source))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func getBuiltInSource(name string) (*models.SHUBSource, bool) {
	trimmedName := strings.TrimSpace(name)
	for _, source := range builtinCatalog {
		if source.Name == trimmedName {
			return cloneSourceRecord(source), true
		}
	}
	return nil, false
}

func cloneSourceRecord(source *models.SHUBSource) *models.SHUBSource {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}
