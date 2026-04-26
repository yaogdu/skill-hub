package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	yaml "gopkg.in/yaml.v3"
)

func ParseDir(dir string) (*models.SkillDocument, error) {
	return ParseFile(filepath.Join(dir, models.SkillFileName))
}

func ParseFile(path string) (*models.SkillDocument, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SKILL.md: %w", err)
	}

	frontmatterContent, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, err
	}

	var frontmatter models.SkillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterContent), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse SKILL.md frontmatter: %w", err)
	}
	if strings.TrimSpace(frontmatter.Name) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required field: name")
	}
	if strings.TrimSpace(frontmatter.Description) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required field: description")
	}

	return &models.SkillDocument{
		Path:        path,
		Body:        body,
		Frontmatter: frontmatter,
	}, nil
}

func splitFrontmatter(content string) (string, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimPrefix(normalized, "\ufeff")
	if strings.TrimSpace(normalized) == "" {
		return "", "", fmt.Errorf("SKILL.md is empty")
	}

	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", fmt.Errorf("SKILL.md missing YAML frontmatter delimited by ---")
	}

	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) != "---" {
			continue
		}

		frontmatter := strings.Join(lines[1:index], "\n")
		body := strings.Join(lines[index+1:], "\n")
		body = strings.TrimLeft(body, "\n")
		return frontmatter, body, nil
	}

	return "", "", fmt.Errorf("SKILL.md missing YAML frontmatter delimited by ---")
}
