package models

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	apiv0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
	"github.com/modelcontextprotocol/registry/pkg/model"
	yaml "gopkg.in/yaml.v3"
)

const (
	ShubSkillSchemaVersion = "shub.skill/v1alpha1"
	ShubAssetSchemaVersion = "shub.asset/v1alpha1"
	SkillFileName          = "SKILL.md"
)

const (
	assetLegacyAgentMetadataKey  = "legacyAgent"
	assetLegacyServerMetadataKey = "legacyServer"
)

type assetLegacyAgentMetadata struct {
	Agent         AgentJSON     `json:"agent"`
	MCPServerRefs []RegistryRef `json:"mcpServerRefs,omitempty"`
	SkillRefs     []RegistryRef `json:"skillRefs,omitempty"`
	PromptRefs    []RegistryRef `json:"promptRefs,omitempty"`
}

type assetLegacyServerMetadata struct {
	Server apiv0.ServerJSON `json:"server"`
}

type AssetCategory string

const (
	AssetCategoryPrompt AssetCategory = "prompt"
	AssetCategoryAgent  AssetCategory = "agent"
	AssetCategoryMCP    AssetCategory = "mcp"
)

func (category AssetCategory) IsValid() bool {
	switch category {
	case AssetCategoryPrompt, AssetCategoryAgent, AssetCategoryMCP:
		return true
	default:
		return false
	}
}

type Asset struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Version      string           `json:"version"`
	Category     AssetCategory    `json:"category"`
	AllowedTools []string         `json:"allowedTools,omitempty"`
	SourceSkill  AssetSourceSkill `json:"sourceSkill"`
	Manifest     AssetManifest    `json:"manifest"`
	Source       *AssetSource     `json:"source,omitempty"`
	Status       string           `json:"status,omitempty"`
}

type AssetSource struct {
	RepositoryURL string `json:"repositoryUrl,omitempty"`
	Commit        string `json:"commit,omitempty"`
	PackageType   string `json:"packageType,omitempty"`
	PackageRef    string `json:"packageRef,omitempty"`
}

type AssetManifest struct {
	SchemaVersion string            `json:"schemaVersion" yaml:"schemaVersion"`
	ID            string            `json:"id" yaml:"id"`
	Category      AssetCategory     `json:"category" yaml:"category"`
	Name          string            `json:"name" yaml:"name"`
	Description   string            `json:"description" yaml:"description"`
	Version       string            `json:"version" yaml:"version"`
	AllowedTools  []string          `json:"allowedTools,omitempty" yaml:"allowedTools,omitempty"`
	SourceSkill   AssetSourceSkill  `json:"sourceSkill" yaml:"sourceSkill"`
	Entry         AssetEntry        `json:"entry" yaml:"entry"`
	Runtime       AssetRuntime      `json:"runtime" yaml:"runtime"`
	Dependencies  AssetDependencies `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Exports       []AssetExport     `json:"exports,omitempty" yaml:"exports,omitempty"`
	Hooks         AssetHooks        `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type AssetSourceSkill struct {
	Path       string `json:"path" yaml:"path"`
	Body       string `json:"body,omitempty" yaml:"body,omitempty"`
	BodyFormat string `json:"bodyFormat" yaml:"bodyFormat"`
}

type AssetEntry struct {
	Kind string   `json:"kind" yaml:"kind"`
	Path string   `json:"path" yaml:"path"`
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
}

type AssetRuntime struct {
	Type    string        `json:"type" yaml:"type"`
	Version string        `json:"version,omitempty" yaml:"version,omitempty"`
	Install *AssetInstall `json:"install,omitempty" yaml:"install,omitempty"`
}

type AssetInstall struct {
	Strategy string `json:"strategy" yaml:"strategy"`
	Path     string `json:"path,omitempty" yaml:"path,omitempty"`
	Lockfile string `json:"lockfile,omitempty" yaml:"lockfile,omitempty"`
}

type AssetDependencies struct {
	Prompts []AssetDependencyRef `json:"prompts,omitempty" yaml:"prompts,omitempty"`
	Skills  []AssetDependencyRef `json:"skills,omitempty" yaml:"skills,omitempty"`
	MCPs    []AssetDependencyRef `json:"mcps,omitempty" yaml:"mcps,omitempty"`
	Agents  []AssetDependencyRef `json:"agents,omitempty" yaml:"agents,omitempty"`
}

type AssetDependencyRef struct {
	ID       string        `json:"id" yaml:"id"`
	Version  string        `json:"version" yaml:"version"`
	Category AssetCategory `json:"category,omitempty" yaml:"category,omitempty"`
}

func (ref *AssetDependencyRef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		parsed, err := ParseAssetDependencyRef(value.Value)
		if err != nil {
			return err
		}
		*ref = parsed
		return nil
	}
	type dependencyRef AssetDependencyRef
	var decoded dependencyRef
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*ref = AssetDependencyRef(decoded)
	return nil
}

func (ref *AssetDependencyRef) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		parsed, err := ParseAssetDependencyRef(raw)
		if err != nil {
			return err
		}
		*ref = parsed
		return nil
	}
	type dependencyRef AssetDependencyRef
	var decoded dependencyRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*ref = AssetDependencyRef(decoded)
	return nil
}

func ParseAssetDependencyRef(value string) (AssetDependencyRef, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return AssetDependencyRef{}, fmt.Errorf("dependency reference is empty")
	}
	id, version, ok := strings.Cut(trimmed, "@")
	if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(version) == "" {
		return AssetDependencyRef{}, fmt.Errorf("dependency reference %q must use <asset-id>@<version>", value)
	}
	if strings.Contains(version, "@") {
		return AssetDependencyRef{}, fmt.Errorf("dependency reference %q has multiple @ separators", value)
	}
	return AssetDependencyRef{ID: strings.TrimSpace(id), Version: strings.TrimSpace(version)}, nil
}

type AssetExport struct {
	Target     string `json:"target" yaml:"target"`
	Mode       string `json:"mode" yaml:"mode"`
	Source     string `json:"source" yaml:"source"`
	TargetPath string `json:"targetPath,omitempty" yaml:"targetPath,omitempty"`
}

type AssetHooks struct {
	PostInstall *AssetCommand `json:"post_install,omitempty" yaml:"post_install,omitempty"`
	PostPull    *AssetCommand `json:"post_pull,omitempty" yaml:"post_pull,omitempty"`
}

type AssetCommand struct {
	Run []string `json:"run" yaml:"run"`
}

type AssetRegistryExtensions struct {
	Status      string    `json:"status"`
	PublishedAt time.Time `json:"publishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	IsLatest    bool      `json:"isLatest"`
}

type AssetResponseMeta struct {
	Official *AssetRegistryExtensions `json:"io.modelcontextprotocol.registry/official,omitempty"`
}

type AssetResponse struct {
	Asset Asset             `json:"asset"`
	Meta  AssetResponseMeta `json:"_meta"`
}

type AssetMetadata struct {
	NextCursor string `json:"nextCursor,omitempty"`
	Count      int    `json:"count"`
}

type AssetPackage struct {
	AssetID     string    `json:"assetId"`
	Version     string    `json:"version"`
	ContentType string    `json:"contentType"`
	SizeBytes   int       `json:"sizeBytes"`
	SHA256      string    `json:"sha256"`
	UploadedAt  time.Time `json:"uploadedAt"`
}

type AssetPackageResponse struct {
	Package     AssetPackage `json:"package"`
	DownloadURL string       `json:"downloadUrl,omitempty"`
}

type AssetPackageDownload struct {
	Package AssetPackage `json:"package"`
	Content []byte       `json:"-"`
}

type AssetListResponse struct {
	Assets   []AssetResponse `json:"assets"`
	Metadata AssetMetadata   `json:"metadata"`
}

type AssetPublishRequest struct {
	Manifest AssetManifest `json:"manifest"`
	Source   *AssetSource  `json:"source,omitempty"`
}

func (request AssetPublishRequest) ToAsset() (*Asset, error) {
	manifest, err := normalizeAssetManifest(request.Manifest)
	if err != nil {
		return nil, err
	}

	return &Asset{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Version:      manifest.Version,
		Category:     manifest.Category,
		AllowedTools: cloneStrings(manifest.AllowedTools),
		SourceSkill:  manifest.SourceSkill,
		Manifest:     manifest,
		Source:       cloneAssetSource(request.Source),
	}, nil
}

func (request AssetPublishRequest) ToSkillJSON() (*SkillJSON, error) {
	asset, err := request.ToAsset()
	if err != nil {
		return nil, err
	}

	payload := &SkillJSON{
		Name:        asset.Name,
		Title:       asset.ID,
		Category:    string(asset.Category),
		Description: asset.Description,
		Version:     asset.Version,
		SHUB: &SkillSHUBMetadata{
			SchemaVersion: ShubAssetSchemaVersion,
			AssetID:       asset.ID,
			Category:      asset.Category,
			Manifest:      cloneAssetManifest(asset.Manifest),
			Source:        cloneAssetSource(asset.Source),
		},
	}

	if asset.Source != nil {
		if strings.TrimSpace(asset.Source.RepositoryURL) != "" {
			payload.Repository = &SkillRepository{URL: asset.Source.RepositoryURL, Source: "git"}
		}
		if pkg := assetSourceToSkillPackage(asset.Version, *asset.Source); pkg != nil {
			payload.Packages = append(payload.Packages, *pkg)
		}
	}

	return payload, nil
}

type SkillDocument struct {
	Path        string           `json:"path"`
	Body        string           `json:"body"`
	Frontmatter SkillFrontmatter `json:"frontmatter"`
}

type SkillFrontmatter struct {
	Name         string               `json:"name" yaml:"name"`
	Description  string               `json:"description" yaml:"description"`
	Version      string               `json:"version,omitempty" yaml:"version,omitempty"`
	AllowedTools []string             `json:"allowed-tools,omitempty" yaml:"allowed-tools,omitempty"`
	Shub         SkillFrontmatterShub `json:"shub,omitempty" yaml:"shub,omitempty"`
}

type SkillFrontmatterShub struct {
	SchemaVersion string            `json:"schemaVersion,omitempty" yaml:"schemaVersion,omitempty"`
	ID            string            `json:"id,omitempty" yaml:"id,omitempty"`
	Category      AssetCategory     `json:"category,omitempty" yaml:"category,omitempty"`
	Entry         AssetEntry        `json:"entry,omitempty" yaml:"entry,omitempty"`
	Runtime       AssetRuntime      `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Dependencies  AssetDependencies `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Exports       []AssetExport     `json:"exports,omitempty" yaml:"exports,omitempty"`
	Hooks         AssetHooks        `json:"hooks,omitempty" yaml:"hooks,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (document SkillDocument) ToAssetManifest() (*AssetManifest, error) {
	if strings.TrimSpace(document.Frontmatter.Name) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required field: name")
	}
	if strings.TrimSpace(document.Frontmatter.Description) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required field: description")
	}
	if strings.TrimSpace(document.Frontmatter.Version) == "" {
		return nil, fmt.Errorf("SKILL.md frontmatter missing required field: version")
	}
	if err := document.Frontmatter.Shub.validate(); err != nil {
		return nil, err
	}

	manifest := &AssetManifest{
		SchemaVersion: ShubAssetSchemaVersion,
		ID:            document.Frontmatter.Shub.ID,
		Category:      document.Frontmatter.Shub.Category,
		Name:          document.Frontmatter.Name,
		Description:   document.Frontmatter.Description,
		Version:       document.Frontmatter.Version,
		AllowedTools:  cloneStrings(document.Frontmatter.AllowedTools),
		SourceSkill: AssetSourceSkill{
			Path:       SkillFileName,
			Body:       document.Body,
			BodyFormat: "markdown",
		},
		Entry:        document.Frontmatter.Shub.Entry,
		Runtime:      document.Frontmatter.Shub.Runtime,
		Dependencies: cloneAssetDependencies(document.Frontmatter.Shub.Dependencies),
		Exports:      cloneAssetExports(document.Frontmatter.Shub.Exports),
		Hooks:        cloneAssetHooks(document.Frontmatter.Shub.Hooks),
		Metadata:     cloneMap(document.Frontmatter.Shub.Metadata),
	}

	if err := validateManifestSemantics(*manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

func (document SkillDocument) ToAsset() (*Asset, error) {
	manifest, err := document.ToAssetManifest()
	if err != nil {
		return nil, err
	}

	return &Asset{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Version:      manifest.Version,
		Category:     manifest.Category,
		AllowedTools: cloneStrings(manifest.AllowedTools),
		SourceSkill:  manifest.SourceSkill,
		Manifest:     *manifest,
	}, nil
}

func (frontmatter SkillFrontmatterShub) validate() error {
	if strings.TrimSpace(frontmatter.SchemaVersion) == "" {
		return fmt.Errorf("SKILL.md frontmatter missing required field: shub.schemaVersion")
	}
	if frontmatter.SchemaVersion != ShubSkillSchemaVersion {
		return fmt.Errorf("unsupported shub.schemaVersion: %s", frontmatter.SchemaVersion)
	}
	if strings.TrimSpace(frontmatter.ID) == "" {
		return fmt.Errorf("SKILL.md frontmatter missing required field: shub.id")
	}
	if !frontmatter.Category.IsValid() {
		return fmt.Errorf("SKILL.md frontmatter has invalid shub.category: %s", frontmatter.Category)
	}
	if strings.TrimSpace(frontmatter.Entry.Kind) == "" {
		return fmt.Errorf("SKILL.md frontmatter missing required field: shub.entry.kind")
	}
	if strings.TrimSpace(frontmatter.Entry.Path) == "" {
		return fmt.Errorf("SKILL.md frontmatter missing required field: shub.entry.path")
	}
	if strings.TrimSpace(frontmatter.Runtime.Type) == "" {
		return fmt.Errorf("SKILL.md frontmatter missing required field: shub.runtime.type")
	}
	return nil
}

func validateManifestSemantics(manifest AssetManifest) error {
	switch manifest.Category {
	case AssetCategoryPrompt:
		if manifest.Entry.Kind != "skill-body" {
			return fmt.Errorf("prompt assets must use entry.kind=skill-body")
		}
		if manifest.Runtime.Type != "none" {
			return fmt.Errorf("prompt assets must use runtime.type=none")
		}
	case AssetCategoryMCP:
		if manifest.Entry.Kind != "mcp-config" && manifest.Entry.Kind != "command" {
			return fmt.Errorf("mcp assets must use entry.kind=mcp-config or command")
		}
	}
	if err := validateDependencies(manifest.Dependencies); err != nil {
		return err
	}
	return nil
}

func validateDependencies(dependencies AssetDependencies) error {
	cases := []struct {
		field    string
		category AssetCategory
		refs     []AssetDependencyRef
	}{
		{field: "dependencies.prompts", category: AssetCategoryPrompt, refs: dependencies.Prompts},
		{field: "dependencies.skills", refs: dependencies.Skills},
		{field: "dependencies.mcps", category: AssetCategoryMCP, refs: dependencies.MCPs},
		{field: "dependencies.agents", category: AssetCategoryAgent, refs: dependencies.Agents},
	}
	seen := make(map[string]struct{})
	for _, tc := range cases {
		for index, ref := range tc.refs {
			field := fmt.Sprintf("%s[%d]", tc.field, index)
			id := strings.TrimSpace(ref.ID)
			version := strings.TrimSpace(ref.Version)
			if id == "" {
				return fmt.Errorf("%s.id is required", field)
			}
			if version == "" {
				return fmt.Errorf("%s.version is required", field)
			}
			if strings.EqualFold(version, "latest") {
				return fmt.Errorf("%s.version must be pinned, got latest", field)
			}
			if ref.Category.IsValid() && tc.category != "" && ref.Category != tc.category {
				return fmt.Errorf("%s.category must be %s, got %s", field, tc.category, ref.Category)
			}
			key := id + "@" + version
			if _, ok := seen[key]; ok {
				return fmt.Errorf("duplicate dependency reference: %s", key)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func normalizeAssetManifest(manifest AssetManifest) (AssetManifest, error) {
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		manifest.SchemaVersion = ShubAssetSchemaVersion
	}
	if manifest.SchemaVersion != ShubAssetSchemaVersion {
		return AssetManifest{}, fmt.Errorf("unsupported manifest.schemaVersion: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return AssetManifest{}, fmt.Errorf("asset manifest missing required field: id")
	}
	if !manifest.Category.IsValid() {
		return AssetManifest{}, fmt.Errorf("asset manifest has invalid category: %s", manifest.Category)
	}
	if strings.TrimSpace(manifest.Name) == "" {
		return AssetManifest{}, fmt.Errorf("asset manifest missing required field: name")
	}
	if strings.TrimSpace(manifest.Description) == "" {
		return AssetManifest{}, fmt.Errorf("asset manifest missing required field: description")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return AssetManifest{}, fmt.Errorf("asset manifest missing required field: version")
	}
	if strings.TrimSpace(manifest.SourceSkill.Path) == "" {
		manifest.SourceSkill.Path = SkillFileName
	}
	if strings.TrimSpace(manifest.SourceSkill.BodyFormat) == "" {
		manifest.SourceSkill.BodyFormat = "markdown"
	}
	if err := validateManifestSemantics(manifest); err != nil {
		return AssetManifest{}, err
	}
	return manifest, nil
}

func PromptResponseFromAssetResponse(response *AssetResponse) (*PromptResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("asset response is nil")
	}
	if response.Asset.Category != AssetCategoryPrompt {
		return nil, fmt.Errorf("asset response category %s is not prompt", response.Asset.Category)
	}

	content := response.Asset.SourceSkill.Body
	if strings.TrimSpace(content) == "" {
		content = response.Asset.Manifest.SourceSkill.Body
	}

	result := &PromptResponse{Prompt: PromptJSON{
		Name:        response.Asset.Name,
		Description: response.Asset.Description,
		Version:     response.Asset.Version,
		Content:     content,
	}}
	if response.Meta.Official != nil {
		result.Meta.Official = &PromptRegistryExtensions{
			Status:      response.Meta.Official.Status,
			PublishedAt: response.Meta.Official.PublishedAt,
			UpdatedAt:   response.Meta.Official.UpdatedAt,
			IsLatest:    response.Meta.Official.IsLatest,
		}
	}
	return result, nil
}

func AgentResponseFromAssetResponse(response *AssetResponse) (*AgentResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("asset response is nil")
	}
	if response.Asset.Category != AssetCategoryAgent {
		return nil, fmt.Errorf("asset response category %s is not agent", response.Asset.Category)
	}

	asset := response.Asset
	compat, _ := extractLegacyAgentMetadata(asset.Manifest.Metadata)
	result := &AgentResponse{}
	if compat != nil {
		result.Agent = compat.Agent
		result.MCPServerRefs = cloneRegistryRefs(compat.MCPServerRefs)
		result.SkillRefs = cloneRegistryRefs(compat.SkillRefs)
		result.PromptRefs = cloneRegistryRefs(compat.PromptRefs)
	}

	if strings.TrimSpace(result.Agent.Name) == "" {
		result.Agent.Name = firstNonEmpty(strings.TrimSpace(asset.Name), strings.TrimSpace(asset.ID))
	}
	if strings.TrimSpace(result.Agent.Description) == "" {
		result.Agent.Description = strings.TrimSpace(asset.Description)
	}
	if strings.TrimSpace(result.Agent.Version) == "" {
		result.Agent.Version = strings.TrimSpace(asset.Version)
	}
	if strings.TrimSpace(result.Agent.AgentManifest.Version) == "" {
		result.Agent.AgentManifest.Version = result.Agent.Version
	}
	if strings.TrimSpace(result.Agent.Title) == "" {
		result.Agent.Title = titleFromSkillBody(assetSkillBody(asset))
	}
	if result.Agent.Repository == nil {
		if repositoryURL := agentRepositoryURLFromAsset(asset); strings.TrimSpace(repositoryURL) != "" {
			result.Agent.Repository = &model.Repository{URL: repositoryURL, Source: "git"}
		}
	}
	if len(result.Agent.Packages) == 0 {
		if pkg := agentPackageFromAsset(asset); pkg != nil {
			result.Agent.Packages = append(result.Agent.Packages, *pkg)
		}
	}
	if strings.TrimSpace(result.Agent.Image) == "" {
		result.Agent.Image = agentImageFromAsset(asset)
	}
	if strings.TrimSpace(result.Agent.Language) == "" {
		result.Agent.Language = agentLanguageFromRuntime(asset.Manifest.Runtime.Type)
	}
	if len(result.Agent.Remotes) == 0 {
		if remote := agentRemoteFromEntry(asset.Manifest.Entry); remote != nil {
			result.Agent.Remotes = []model.Transport{*remote}
		}
	}
	if len(result.MCPServerRefs) == 0 {
		result.MCPServerRefs = cloneRegistryRefs(result.Agent.ExtractMCPServerRefs())
	}
	if response.Meta.Official != nil {
		result.Meta.Official = &AgentRegistryExtensions{
			Status:      response.Meta.Official.Status,
			PublishedAt: response.Meta.Official.PublishedAt,
			UpdatedAt:   response.Meta.Official.UpdatedAt,
			IsLatest:    response.Meta.Official.IsLatest,
		}
		if strings.TrimSpace(result.Agent.Status) == "" {
			result.Agent.Status = response.Meta.Official.Status
		}
	}
	return result, nil
}

func SkillResponseFromAssetResponse(response *AssetResponse) (*SkillResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("asset response is nil")
	}
	result := &SkillResponse{Skill: SkillJSON{
		Name:        response.Asset.Name,
		Title:       response.Asset.ID,
		Category:    string(response.Asset.Category),
		Description: response.Asset.Description,
		Version:     response.Asset.Version,
		SHUB: &SkillSHUBMetadata{
			SchemaVersion: ShubAssetSchemaVersion,
			AssetID:       response.Asset.ID,
			Category:      response.Asset.Category,
			Manifest:      cloneAssetManifest(response.Asset.Manifest),
			Source:        cloneAssetSource(response.Asset.Source),
		},
	}}
	if response.Asset.Source != nil {
		if strings.TrimSpace(response.Asset.Source.RepositoryURL) != "" {
			result.Skill.Repository = &SkillRepository{URL: response.Asset.Source.RepositoryURL, Source: "git"}
		}
		if pkg := assetSourceToSkillPackage(response.Asset.Version, *response.Asset.Source); pkg != nil {
			result.Skill.Packages = append(result.Skill.Packages, *pkg)
		}
	}
	if response.Meta.Official != nil {
		result.Meta.Official = &SkillRegistryExtensions{
			Status:      response.Meta.Official.Status,
			PublishedAt: response.Meta.Official.PublishedAt,
			UpdatedAt:   response.Meta.Official.UpdatedAt,
			IsLatest:    response.Meta.Official.IsLatest,
		}
	}
	return result, nil
}

func ServerResponseFromAssetResponse(response *AssetResponse) (*apiv0.ServerResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("asset response is nil")
	}
	if response.Asset.Category != AssetCategoryMCP {
		return nil, fmt.Errorf("asset response category %s is not mcp", response.Asset.Category)
	}

	asset := response.Asset
	compat, _ := extractLegacyServerMetadata(asset.Manifest.Metadata)
	result := &apiv0.ServerResponse{}
	if compat != nil {
		result.Server = compat.Server
	}

	if strings.TrimSpace(result.Server.Schema) == "" {
		result.Server.Schema = model.CurrentSchemaURL
	}
	if strings.TrimSpace(result.Server.Name) == "" {
		result.Server.Name = firstNonEmpty(strings.TrimSpace(asset.ID), strings.TrimSpace(asset.Name))
	}
	if strings.TrimSpace(result.Server.Description) == "" {
		result.Server.Description = firstNonEmpty(strings.TrimSpace(asset.Description), strings.TrimSpace(asset.Name), strings.TrimSpace(asset.ID))
	}
	if strings.TrimSpace(result.Server.Version) == "" {
		result.Server.Version = strings.TrimSpace(asset.Version)
	}
	if strings.TrimSpace(result.Server.Title) == "" {
		if strings.TrimSpace(asset.Name) != "" && strings.TrimSpace(asset.Name) != strings.TrimSpace(asset.ID) {
			result.Server.Title = strings.TrimSpace(asset.Name)
		} else if title := titleFromSkillBody(assetSkillBody(asset)); strings.TrimSpace(title) != "" && title != strings.TrimSpace(asset.ID) {
			result.Server.Title = title
		}
	}
	if result.Server.Repository == nil {
		if repositoryURL := serverRepositoryURLFromAsset(asset); strings.TrimSpace(repositoryURL) != "" {
			result.Server.Repository = &model.Repository{URL: repositoryURL, Source: "git"}
		}
	}
	if len(result.Server.Packages) == 0 {
		if pkg := serverPackageFromAsset(asset); pkg != nil {
			result.Server.Packages = append(result.Server.Packages, *pkg)
		}
	}
	if len(result.Server.Remotes) == 0 {
		if remote := serverRemoteFromAsset(asset.Manifest.Entry); remote != nil {
			result.Server.Remotes = []model.Transport{*remote}
		}
	}
	if response.Meta.Official != nil {
		result.Meta.Official = &apiv0.RegistryExtensions{
			Status:      model.Status(response.Meta.Official.Status),
			PublishedAt: response.Meta.Official.PublishedAt,
			UpdatedAt:   response.Meta.Official.UpdatedAt,
			IsLatest:    response.Meta.Official.IsLatest,
		}
	}
	return result, nil
}

func AssetResponseFromServerResponse(response *apiv0.ServerResponse, readme string) (*AssetResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("server response is nil")
	}
	if strings.TrimSpace(response.Server.Name) == "" {
		return nil, fmt.Errorf("server response missing server name")
	}
	if strings.TrimSpace(response.Server.Version) == "" {
		return nil, fmt.Errorf("server response missing server version")
	}

	description := strings.TrimSpace(response.Server.Description)
	if description == "" {
		description = response.Server.Name
	}
	name := firstNonEmpty(strings.TrimSpace(response.Server.Title), strings.TrimSpace(response.Server.Name))
	if strings.TrimSpace(name) == "" {
		name = response.Server.Name
	}
	sourceSkill := AssetSourceSkill{
		Path:       SkillFileName,
		Body:       renderServerSkillBody(response.Server, readme),
		BodyFormat: "markdown",
	}
	manifest := AssetManifest{
		SchemaVersion: ShubAssetSchemaVersion,
		ID:            response.Server.Name,
		Category:      AssetCategoryMCP,
		Name:          name,
		Description:   description,
		Version:       response.Server.Version,
		SourceSkill:   sourceSkill,
		Entry:         deriveServerAssetEntry(response.Server),
		Runtime:       deriveServerAssetRuntime(response.Server),
		Metadata: map[string]any{
			assetLegacyServerMetadataKey: assetLegacyServerMetadata{Server: response.Server},
		},
	}
	asset := Asset{
		ID:          response.Server.Name,
		Name:        name,
		Description: description,
		Version:     response.Server.Version,
		Category:    AssetCategoryMCP,
		SourceSkill: sourceSkill,
		Manifest:    manifest,
		Source:      inferAssetSourceFromServer(response.Server),
	}
	if response.Meta.Official != nil {
		asset.Status = string(response.Meta.Official.Status)
	}
	return &AssetResponse{
		Asset: asset,
		Meta:  AssetResponseMeta{Official: serverExtensionsToAssetExtensions(response.Meta.Official)},
	}, nil
}

func AssetResponseFromAgentResponse(response *AgentResponse) (*AssetResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("agent response is nil")
	}
	if strings.TrimSpace(response.Agent.Name) == "" {
		return nil, fmt.Errorf("agent response missing agent name")
	}
	if strings.TrimSpace(response.Agent.Version) == "" {
		return nil, fmt.Errorf("agent response missing agent version")
	}

	description := strings.TrimSpace(response.Agent.Description)
	if description == "" {
		description = response.Agent.Name
	}
	sourceSkill := AssetSourceSkill{
		Path:       SkillFileName,
		Body:       renderAgentSkillBody(response.Agent),
		BodyFormat: "markdown",
	}
	manifest := AssetManifest{
		SchemaVersion: ShubAssetSchemaVersion,
		ID:            response.Agent.Name,
		Category:      AssetCategoryAgent,
		Name:          response.Agent.Name,
		Description:   description,
		Version:       response.Agent.Version,
		SourceSkill:   sourceSkill,
		Entry:         deriveAgentAssetEntry(response.Agent),
		Runtime:       deriveAgentAssetRuntime(response.Agent),
		Metadata: map[string]any{
			assetLegacyAgentMetadataKey: assetLegacyAgentMetadata{
				Agent:         response.Agent,
				MCPServerRefs: cloneRegistryRefs(response.MCPServerRefs),
				SkillRefs:     cloneRegistryRefs(response.SkillRefs),
				PromptRefs:    cloneRegistryRefs(response.PromptRefs),
			},
		},
	}
	asset := Asset{
		ID:          response.Agent.Name,
		Name:        response.Agent.Name,
		Description: description,
		Version:     response.Agent.Version,
		Category:    AssetCategoryAgent,
		SourceSkill: sourceSkill,
		Manifest:    manifest,
		Source:      inferAssetSourceFromAgent(response.Agent),
	}
	if response.Meta.Official != nil {
		asset.Status = response.Meta.Official.Status
	} else if strings.TrimSpace(response.Agent.Status) != "" {
		asset.Status = response.Agent.Status
	}
	return &AssetResponse{
		Asset: asset,
		Meta:  AssetResponseMeta{Official: agentExtensionsToAssetExtensions(response.Meta.Official)},
	}, nil
}

func AssetResponseFromPromptResponse(response *PromptResponse) (*AssetResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("prompt response is nil")
	}
	if strings.TrimSpace(response.Prompt.Name) == "" {
		return nil, fmt.Errorf("prompt response missing prompt name")
	}
	if strings.TrimSpace(response.Prompt.Version) == "" {
		return nil, fmt.Errorf("prompt response missing prompt version")
	}

	description := strings.TrimSpace(response.Prompt.Description)
	if description == "" {
		description = response.Prompt.Name
	}
	asset := Asset{
		ID:          response.Prompt.Name,
		Name:        response.Prompt.Name,
		Description: description,
		Version:     response.Prompt.Version,
		Category:    AssetCategoryPrompt,
		SourceSkill: AssetSourceSkill{
			Path:       SkillFileName,
			Body:       response.Prompt.Content,
			BodyFormat: "markdown",
		},
		Manifest: AssetManifest{
			SchemaVersion: ShubAssetSchemaVersion,
			ID:            response.Prompt.Name,
			Category:      AssetCategoryPrompt,
			Name:          response.Prompt.Name,
			Description:   description,
			Version:       response.Prompt.Version,
			SourceSkill: AssetSourceSkill{
				Path:       SkillFileName,
				Body:       response.Prompt.Content,
				BodyFormat: "markdown",
			},
			Entry:   AssetEntry{Kind: "skill-body", Path: SkillFileName},
			Runtime: AssetRuntime{Type: "none"},
		},
	}
	if response.Meta.Official != nil {
		asset.Status = response.Meta.Official.Status
	}
	return &AssetResponse{
		Asset: asset,
		Meta:  AssetResponseMeta{Official: promptExtensionsToAssetExtensions(response.Meta.Official)},
	}, nil
}

func (request AssetPublishRequest) ToServerJSON() (*apiv0.ServerJSON, string, error) {
	asset, err := request.ToAsset()
	if err != nil {
		return nil, "", err
	}
	if asset.Category != AssetCategoryMCP {
		return nil, "", fmt.Errorf("asset manifest category %s is not mcp", asset.Category)
	}
	response, err := ServerResponseFromAssetResponse(&AssetResponse{Asset: *asset})
	if err != nil {
		return nil, "", err
	}
	return &response.Server, assetSkillBody(*asset), nil
}

func AssetResponseFromSkillResponse(response *SkillResponse) (*AssetResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("skill response is nil")
	}
	if response.Skill.SHUB == nil || response.Skill.SHUB.Manifest == nil {
		return nil, fmt.Errorf("skill response does not contain SHUB asset metadata")
	}

	manifest := *response.Skill.SHUB.Manifest
	if strings.TrimSpace(manifest.ID) == "" {
		manifest.ID = response.Skill.SHUB.AssetID
	}
	if strings.TrimSpace(manifest.ID) == "" {
		return nil, fmt.Errorf("skill response SHUB metadata missing asset id")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		manifest.Name = response.Skill.Name
	}
	if strings.TrimSpace(manifest.Description) == "" {
		manifest.Description = response.Skill.Description
	}
	if strings.TrimSpace(manifest.Version) == "" {
		manifest.Version = response.Skill.Version
	}
	if !manifest.Category.IsValid() {
		manifest.Category = response.Skill.SHUB.Category
	}
	if !manifest.Category.IsValid() {
		manifest.Category = AssetCategoryPrompt
	}
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		manifest.SchemaVersion = ShubAssetSchemaVersion
	}
	if strings.TrimSpace(manifest.SourceSkill.Path) == "" {
		manifest.SourceSkill.Path = SkillFileName
	}
	if strings.TrimSpace(manifest.SourceSkill.BodyFormat) == "" {
		manifest.SourceSkill.BodyFormat = "markdown"
	}
	if err := validateManifestSemantics(manifest); err != nil {
		return nil, err
	}

	source := cloneAssetSource(response.Skill.SHUB.Source)
	if source == nil {
		source = inferAssetSourceFromSkill(response.Skill)
	}

	asset := Asset{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Description:  manifest.Description,
		Version:      manifest.Version,
		Category:     manifest.Category,
		AllowedTools: cloneStrings(manifest.AllowedTools),
		SourceSkill:  manifest.SourceSkill,
		Manifest:     manifest,
		Source:       source,
	}
	if response.Meta.Official != nil {
		asset.Status = response.Meta.Official.Status
	}

	return &AssetResponse{
		Asset: asset,
		Meta: AssetResponseMeta{
			Official: cloneAssetRegistryExtensions(response.Meta.Official),
		},
	}, nil
}

func extractLegacyServerMetadata(metadata map[string]any) (*assetLegacyServerMetadata, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	raw, ok := metadata[assetLegacyServerMetadataKey]
	if !ok || raw == nil {
		return nil, false
	}
	var compat assetLegacyServerMetadata
	if !decodeAssetMetadata(raw, &compat) {
		return nil, false
	}
	return &compat, true
}

func extractLegacyAgentMetadata(metadata map[string]any) (*assetLegacyAgentMetadata, bool) {
	if len(metadata) == 0 {
		return nil, false
	}
	raw, ok := metadata[assetLegacyAgentMetadataKey]
	if !ok || raw == nil {
		return nil, false
	}
	var compat assetLegacyAgentMetadata
	if !decodeAssetMetadata(raw, &compat) {
		return nil, false
	}
	return &compat, true
}

func decodeAssetMetadata(value any, target any) bool {
	payload, err := json.Marshal(value)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return false
	}
	return true
}

func agentExtensionsToAssetExtensions(meta *AgentRegistryExtensions) *AssetRegistryExtensions {
	if meta == nil {
		return nil
	}
	return &AssetRegistryExtensions{
		Status:      meta.Status,
		PublishedAt: meta.PublishedAt,
		UpdatedAt:   meta.UpdatedAt,
		IsLatest:    meta.IsLatest,
	}
}

func serverExtensionsToAssetExtensions(meta *apiv0.RegistryExtensions) *AssetRegistryExtensions {
	if meta == nil {
		return nil
	}
	return &AssetRegistryExtensions{
		Status:      string(meta.Status),
		PublishedAt: meta.PublishedAt,
		UpdatedAt:   meta.UpdatedAt,
		IsLatest:    meta.IsLatest,
	}
}

func renderServerSkillBody(server apiv0.ServerJSON, readme string) string {
	if strings.TrimSpace(readme) != "" {
		return strings.TrimSpace(readme)
	}

	title := firstNonEmpty(strings.TrimSpace(server.Title), strings.TrimSpace(server.Name))
	sections := make([]string, 0, 3)
	if title != "" {
		sections = append(sections, "# "+title)
	}
	if description := strings.TrimSpace(server.Description); description != "" {
		sections = append(sections, description)
	}

	details := make([]string, 0, 5)
	if server.Repository != nil && strings.TrimSpace(server.Repository.URL) != "" {
		details = append(details, "- Repository: "+strings.TrimSpace(server.Repository.URL))
	}
	if websiteURL := strings.TrimSpace(server.WebsiteURL); websiteURL != "" {
		details = append(details, "- Website: "+websiteURL)
	}
	if pkg := firstServerPackage(server); pkg != nil {
		details = append(details, "- Package: "+strings.TrimSpace(pkg.Identifier))
	}
	if len(server.Remotes) > 0 && strings.TrimSpace(server.Remotes[0].URL) != "" {
		details = append(details, "- Remote: "+strings.TrimSpace(server.Remotes[0].URL))
	}
	if len(details) > 0 {
		sections = append(sections, "## MCP\n"+strings.Join(details, "\n"))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func deriveServerAssetEntry(server apiv0.ServerJSON) AssetEntry {
	if len(server.Remotes) > 0 && strings.TrimSpace(server.Remotes[0].URL) != "" {
		return AssetEntry{Kind: "mcp-config", Path: strings.TrimSpace(server.Remotes[0].URL)}
	}
	if pkg := firstServerPackage(server); pkg != nil && strings.TrimSpace(pkg.Identifier) != "" {
		return AssetEntry{Kind: "mcp-config", Path: strings.TrimSpace(pkg.Identifier)}
	}
	if server.Repository != nil && strings.TrimSpace(server.Repository.URL) != "" {
		return AssetEntry{Kind: "mcp-config", Path: strings.TrimSpace(server.Repository.URL)}
	}
	return AssetEntry{Kind: "mcp-config", Path: "server.json"}
}

func deriveServerAssetRuntime(server apiv0.ServerJSON) AssetRuntime {
	runtimeType := ""
	if pkg := firstServerPackage(server); pkg != nil {
		runtimeType = serverRuntimeTypeFromPackage(*pkg)
	}
	if runtimeType == "" {
		if len(server.Remotes) > 0 && strings.TrimSpace(server.Remotes[0].URL) != "" {
			runtimeType = "remote"
		} else {
			runtimeType = "mcp"
		}
	}
	return AssetRuntime{Type: runtimeType}
}

func inferAssetSourceFromServer(server apiv0.ServerJSON) *AssetSource {
	source := &AssetSource{}
	if server.Repository != nil {
		source.RepositoryURL = strings.TrimSpace(server.Repository.URL)
	}
	if pkg := firstServerPackage(server); pkg != nil {
		source.PackageType = strings.TrimSpace(pkg.RegistryType)
		source.PackageRef = strings.TrimSpace(pkg.Identifier)
	}
	if source.RepositoryURL == "" && source.PackageRef == "" {
		return nil
	}
	return source
}

func firstServerPackage(server apiv0.ServerJSON) *model.Package {
	for _, pkg := range server.Packages {
		if strings.TrimSpace(pkg.Identifier) == "" {
			continue
		}
		copy := pkg
		return &copy
	}
	return nil
}

func serverRuntimeTypeFromPackage(pkg model.Package) string {
	if strings.TrimSpace(pkg.RunTimeHint) != "" {
		return strings.TrimSpace(pkg.RunTimeHint)
	}
	if strings.TrimSpace(pkg.RegistryType) != "" {
		return strings.ToLower(strings.TrimSpace(pkg.RegistryType))
	}
	return ""
}

func renderAgentSkillBody(agent AgentJSON) string {
	title := strings.TrimSpace(agent.Title)
	if title == "" {
		title = strings.TrimSpace(agent.Name)
	}

	sections := make([]string, 0, 3)
	if title != "" {
		sections = append(sections, "# "+title)
	}
	if description := strings.TrimSpace(agent.Description); description != "" {
		sections = append(sections, description)
	}

	details := make([]string, 0, 6)
	if framework := strings.TrimSpace(agent.Framework); framework != "" {
		details = append(details, "- Framework: "+framework)
	}
	if language := strings.TrimSpace(agent.Language); language != "" {
		details = append(details, "- Runtime: "+language)
	}
	modelProvider := strings.TrimSpace(agent.ModelProvider)
	modelName := strings.TrimSpace(agent.ModelName)
	if modelProvider != "" || modelName != "" {
		modelValue := strings.Trim(strings.Join([]string{modelProvider, modelName}, "/"), "/")
		if modelValue != "" {
			details = append(details, "- Model: "+modelValue)
		}
	}
	if image := strings.TrimSpace(agent.Image); image != "" {
		details = append(details, "- Image: "+image)
	}
	if agent.Repository != nil && strings.TrimSpace(agent.Repository.URL) != "" {
		details = append(details, "- Repository: "+strings.TrimSpace(agent.Repository.URL))
	}
	if len(agent.Remotes) > 0 && strings.TrimSpace(agent.Remotes[0].URL) != "" {
		details = append(details, "- Remote: "+strings.TrimSpace(agent.Remotes[0].URL))
	}
	if len(details) > 0 {
		sections = append(sections, "## Runtime\n"+strings.Join(details, "\n"))
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func deriveAgentAssetEntry(agent AgentJSON) AssetEntry {
	if image := strings.TrimSpace(agent.Image); image != "" {
		return AssetEntry{Kind: "image", Path: image}
	}
	if pkg := firstAgentPackage(agent); pkg != nil && strings.TrimSpace(pkg.Identifier) != "" {
		kind := strings.ToLower(strings.TrimSpace(pkg.RegistryType))
		if kind == "" {
			kind = "package"
		}
		if kind == "docker" || kind == "oci" {
			kind = "image"
		}
		return AssetEntry{Kind: kind, Path: strings.TrimSpace(pkg.Identifier)}
	}
	if len(agent.Remotes) > 0 && strings.TrimSpace(agent.Remotes[0].URL) != "" {
		return AssetEntry{Kind: "remote", Path: strings.TrimSpace(agent.Remotes[0].URL)}
	}
	if agent.Repository != nil && strings.TrimSpace(agent.Repository.URL) != "" {
		return AssetEntry{Kind: "repository", Path: strings.TrimSpace(agent.Repository.URL)}
	}
	return AssetEntry{Kind: "agent-manifest", Path: "agent.json"}
}

func deriveAgentAssetRuntime(agent AgentJSON) AssetRuntime {
	runtimeType := strings.ToLower(strings.TrimSpace(agent.Language))
	if runtimeType == "" {
		switch {
		case strings.TrimSpace(agent.Image) != "":
			runtimeType = "container"
		case firstAgentPackage(agent) != nil:
			runtimeType = strings.ToLower(strings.TrimSpace(firstAgentPackage(agent).RegistryType))
			if runtimeType == "" {
				runtimeType = "package"
			}
		case strings.TrimSpace(agent.Framework) != "":
			runtimeType = strings.ToLower(strings.TrimSpace(agent.Framework))
		default:
			runtimeType = "agent"
		}
	}
	return AssetRuntime{Type: runtimeType}
}

func inferAssetSourceFromAgent(agent AgentJSON) *AssetSource {
	source := &AssetSource{}
	if agent.Repository != nil {
		source.RepositoryURL = strings.TrimSpace(agent.Repository.URL)
	}
	if pkg := firstAgentPackage(agent); pkg != nil {
		source.PackageType = strings.TrimSpace(pkg.RegistryType)
		source.PackageRef = strings.TrimSpace(pkg.Identifier)
	} else if image := strings.TrimSpace(agent.Image); image != "" {
		source.PackageType = "docker"
		source.PackageRef = image
	}
	if source.RepositoryURL == "" && source.PackageRef == "" {
		return nil
	}
	return source
}

func assetSkillBody(asset Asset) string {
	body := strings.TrimSpace(asset.SourceSkill.Body)
	if body != "" {
		return body
	}
	return strings.TrimSpace(asset.Manifest.SourceSkill.Body)
}

func titleFromSkillBody(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func agentRepositoryURLFromAsset(asset Asset) string {
	if asset.Source != nil && strings.TrimSpace(asset.Source.RepositoryURL) != "" {
		return strings.TrimSpace(asset.Source.RepositoryURL)
	}
	if strings.EqualFold(strings.TrimSpace(asset.Manifest.Entry.Kind), "repository") {
		return strings.TrimSpace(asset.Manifest.Entry.Path)
	}
	return ""
}

func agentPackageFromAsset(asset Asset) *AgentPackageInfo {
	if asset.Source != nil {
		if pkg := assetSourceToAgentPackage(asset.Version, *asset.Source); pkg != nil {
			return pkg
		}
	}
	if image := agentImageFromAsset(asset); image != "" {
		pkg := &AgentPackageInfo{RegistryType: "docker", Identifier: image, Version: asset.Version}
		pkg.Transport.Type = "docker"
		return pkg
	}
	return nil
}

func assetSourceToAgentPackage(version string, source AssetSource) *AgentPackageInfo {
	if strings.TrimSpace(source.PackageRef) == "" {
		return nil
	}
	registryType := strings.TrimSpace(source.PackageType)
	if registryType == "" {
		registryType = "tarball"
	}
	pkg := &AgentPackageInfo{
		RegistryType: registryType,
		Identifier:   source.PackageRef,
		Version:      version,
	}
	switch strings.ToLower(registryType) {
	case "docker", "oci":
		pkg.Transport.Type = "docker"
	default:
		pkg.Transport.Type = registryType
	}
	return pkg
}

func firstAgentPackage(agent AgentJSON) *AgentPackageInfo {
	for _, pkg := range agent.Packages {
		if strings.TrimSpace(pkg.Identifier) == "" {
			continue
		}
		copy := pkg
		return &copy
	}
	return nil
}

func agentImageFromAsset(asset Asset) string {
	if strings.EqualFold(strings.TrimSpace(asset.Manifest.Entry.Kind), "image") {
		return strings.TrimSpace(asset.Manifest.Entry.Path)
	}
	if asset.Source != nil {
		packageType := strings.ToLower(strings.TrimSpace(asset.Source.PackageType))
		if (packageType == "docker" || packageType == "oci") && strings.TrimSpace(asset.Source.PackageRef) != "" {
			return strings.TrimSpace(asset.Source.PackageRef)
		}
	}
	return ""
}

func agentLanguageFromRuntime(runtimeType string) string {
	value := strings.TrimSpace(runtimeType)
	switch strings.ToLower(value) {
	case "", "agent", "container", "docker", "oci", "package", "none":
		return ""
	default:
		return value
	}
}

func serverRepositoryURLFromAsset(asset Asset) string {
	if asset.Source != nil && strings.TrimSpace(asset.Source.RepositoryURL) != "" {
		return strings.TrimSpace(asset.Source.RepositoryURL)
	}
	return ""
}

func serverPackageFromAsset(asset Asset) *model.Package {
	if asset.Source != nil {
		if pkg := assetSourceToServerPackage(asset.Version, *asset.Source, asset.Manifest.Runtime.Type); pkg != nil {
			return pkg
		}
	}
	return nil
}

func assetSourceToServerPackage(version string, source AssetSource, runtimeType string) *model.Package {
	if strings.TrimSpace(source.PackageRef) == "" {
		return nil
	}
	registryType := strings.TrimSpace(source.PackageType)
	pkg := &model.Package{
		RegistryType: registryType,
		Identifier:   strings.TrimSpace(source.PackageRef),
		Version:      strings.TrimSpace(version),
		Transport:    model.Transport{Type: model.TransportTypeStdio},
	}
	if strings.TrimSpace(runtimeType) != "" {
		pkg.RunTimeHint = strings.TrimSpace(runtimeType)
	}
	return pkg
}

func serverRemoteFromAsset(entry AssetEntry) *model.Transport {
	if !strings.EqualFold(strings.TrimSpace(entry.Kind), "mcp-config") {
		return nil
	}
	path := strings.TrimSpace(entry.Path)
	if !looksLikeRemoteURL(path) {
		return nil
	}
	return &model.Transport{Type: model.TransportTypeStreamableHTTP, URL: path}
}

func looksLikeRemoteURL(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://")
}

func agentRemoteFromEntry(entry AssetEntry) *model.Transport {
	if !strings.EqualFold(strings.TrimSpace(entry.Kind), "remote") || strings.TrimSpace(entry.Path) == "" {
		return nil
	}
	return &model.Transport{Type: model.TransportTypeStreamableHTTP, URL: strings.TrimSpace(entry.Path)}
}

func cloneRegistryRefs(refs []RegistryRef) []RegistryRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]RegistryRef, len(refs))
	copy(cloned, refs)
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func promptExtensionsToAssetExtensions(meta *PromptRegistryExtensions) *AssetRegistryExtensions {
	if meta == nil {
		return nil
	}
	return &AssetRegistryExtensions{
		Status:      meta.Status,
		PublishedAt: meta.PublishedAt,
		UpdatedAt:   meta.UpdatedAt,
		IsLatest:    meta.IsLatest,
	}
}

func assetSourceToSkillPackage(version string, source AssetSource) *SkillPackageInfo {
	if strings.TrimSpace(source.PackageRef) == "" {
		return nil
	}

	registryType := strings.TrimSpace(source.PackageType)
	if registryType == "" {
		registryType = "tarball"
	}
	pkg := &SkillPackageInfo{
		RegistryType: registryType,
		Identifier:   source.PackageRef,
		Version:      version,
	}
	switch registryType {
	case "docker":
		pkg.Transport.Type = "docker"
	case "tarball":
		pkg.Transport.Type = "streamable-http"
	default:
		pkg.Transport.Type = registryType
	}
	return pkg
}

func cloneAssetManifest(manifest AssetManifest) *AssetManifest {
	cloned := manifest
	cloned.AllowedTools = cloneStrings(manifest.AllowedTools)
	cloned.Dependencies = cloneAssetDependencies(manifest.Dependencies)
	cloned.Exports = cloneAssetExports(manifest.Exports)
	cloned.Hooks = cloneAssetHooks(manifest.Hooks)
	cloned.Metadata = cloneMap(manifest.Metadata)
	if manifest.Runtime.Install != nil {
		install := *manifest.Runtime.Install
		cloned.Runtime.Install = &install
	}
	return &cloned
}

func cloneAssetSource(source *AssetSource) *AssetSource {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func inferAssetSourceFromSkill(skill SkillJSON) *AssetSource {
	source := &AssetSource{}
	if skill.Repository != nil {
		source.RepositoryURL = skill.Repository.URL
	}
	if len(skill.Packages) > 0 {
		source.PackageType = skill.Packages[0].RegistryType
		source.PackageRef = skill.Packages[0].Identifier
	}
	if source.RepositoryURL == "" && source.PackageRef == "" {
		return nil
	}
	return source
}

func cloneAssetRegistryExtensions(extensions *SkillRegistryExtensions) *AssetRegistryExtensions {
	if extensions == nil {
		return nil
	}
	return &AssetRegistryExtensions{
		Status:      extensions.Status,
		PublishedAt: extensions.PublishedAt,
		UpdatedAt:   extensions.UpdatedAt,
		IsLatest:    extensions.IsLatest,
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneAssetExports(exports []AssetExport) []AssetExport {
	if len(exports) == 0 {
		return nil
	}
	cloned := make([]AssetExport, len(exports))
	copy(cloned, exports)
	return cloned
}

func cloneAssetDependencies(dependencies AssetDependencies) AssetDependencies {
	return AssetDependencies{
		Prompts: cloneAssetDependencyRefs(dependencies.Prompts),
		Skills:  cloneAssetDependencyRefs(dependencies.Skills),
		MCPs:    cloneAssetDependencyRefs(dependencies.MCPs),
		Agents:  cloneAssetDependencyRefs(dependencies.Agents),
	}
}

func cloneAssetDependencyRefs(refs []AssetDependencyRef) []AssetDependencyRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]AssetDependencyRef, len(refs))
	copy(cloned, refs)
	return cloned
}

func cloneAssetHooks(hooks AssetHooks) AssetHooks {
	return AssetHooks{
		PostInstall: cloneAssetCommand(hooks.PostInstall),
		PostPull:    cloneAssetCommand(hooks.PostPull),
	}
}

func cloneAssetCommand(command *AssetCommand) *AssetCommand {
	if command == nil {
		return nil
	}
	return &AssetCommand{Run: cloneStrings(command.Run)}
}

func cloneMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	maps.Copy(cloned, values)
	return cloned
}
