package models

import (
	"testing"

	apiv0 "github.com/modelcontextprotocol/registry/pkg/api/v0"
	"github.com/modelcontextprotocol/registry/pkg/model"
)

func TestAssetPublishRequestToSkillJSON(t *testing.T) {
	request := AssetPublishRequest{
		Manifest: AssetManifest{
			SchemaVersion: ShubAssetSchemaVersion,
			ID:            "arch/java-analyzer",
			Category:      AssetCategoryPrompt,
			Name:          "java-analyzer",
			Description:   "Analyze Java services",
			Version:       "1.2.0",
			AllowedTools:  []string{"Read", "Write"},
			SourceSkill:   AssetSourceSkill{Path: SkillFileName, Body: "# Java Analyzer", BodyFormat: "markdown"},
			Entry:         AssetEntry{Kind: "skill-body", Path: SkillFileName},
			Runtime:       AssetRuntime{Type: "none"},
		},
		Source: &AssetSource{RepositoryURL: "https://gitlab.example.com/arch/java-analyzer", PackageType: "tarball", PackageRef: "https://gitlab.example.com/packages/java-analyzer-1.2.0.tgz"},
	}

	skill, err := request.ToSkillJSON()
	if err != nil {
		t.Fatalf("ToSkillJSON() error = %v", err)
	}
	if skill.SHUB == nil || skill.SHUB.Manifest == nil {
		t.Fatal("SHUB metadata missing from skill payload")
	}
	if skill.Name != "java-analyzer" {
		t.Fatalf("skill.Name = %q, want %q", skill.Name, "java-analyzer")
	}
	if len(skill.Packages) != 1 || skill.Packages[0].RegistryType != "tarball" {
		t.Fatalf("packages = %#v, want tarball package", skill.Packages)
	}
	if skill.Repository == nil || skill.Repository.URL != "https://gitlab.example.com/arch/java-analyzer" {
		t.Fatalf("repository = %#v, want git repository", skill.Repository)
	}
}

func TestAssetPublishRequestToAssetRejectsInvalidPromptManifest(t *testing.T) {
	_, err := (AssetPublishRequest{Manifest: AssetManifest{
		SchemaVersion: ShubAssetSchemaVersion,
		ID:            "arch/java-analyzer",
		Category:      AssetCategoryPrompt,
		Name:          "java-analyzer",
		Description:   "Analyze Java services",
		Version:       "1.2.0",
		SourceSkill:   AssetSourceSkill{Path: SkillFileName, BodyFormat: "markdown"},
		Entry:         AssetEntry{Kind: "command", Path: "run.sh"},
		Runtime:       AssetRuntime{Type: "python"},
	}}).ToAsset()
	if err == nil {
		t.Fatal("ToAsset() error = nil, want validation error")
	}
}

func TestPromptResponseFromAssetResponse(t *testing.T) {
	prompt, err := PromptResponseFromAssetResponse(&AssetResponse{
		Asset: Asset{
			ID:          "acme/welcome-prompt",
			Name:        "welcome-prompt",
			Description: "Welcome system prompt",
			Version:     "1.0.0",
			Category:    AssetCategoryPrompt,
			SourceSkill: AssetSourceSkill{Path: SkillFileName, Body: "You are helpful.", BodyFormat: "markdown"},
			Manifest: AssetManifest{
				SchemaVersion: ShubAssetSchemaVersion,
				ID:            "acme/welcome-prompt",
				Category:      AssetCategoryPrompt,
				Name:          "welcome-prompt",
				Description:   "Welcome system prompt",
				Version:       "1.0.0",
				SourceSkill:   AssetSourceSkill{Path: SkillFileName, Body: "You are helpful.", BodyFormat: "markdown"},
				Entry:         AssetEntry{Kind: "skill-body", Path: SkillFileName},
				Runtime:       AssetRuntime{Type: "none"},
			},
		},
		Meta: AssetResponseMeta{Official: &AssetRegistryExtensions{Status: "active", IsLatest: true}},
	})
	if err != nil {
		t.Fatalf("PromptResponseFromAssetResponse() error = %v", err)
	}
	if prompt.Prompt.Name != "welcome-prompt" {
		t.Fatalf("prompt name = %q, want %q", prompt.Prompt.Name, "welcome-prompt")
	}
	if prompt.Prompt.Content != "You are helpful." {
		t.Fatalf("prompt content = %q, want %q", prompt.Prompt.Content, "You are helpful.")
	}
	if prompt.Meta.Official == nil || !prompt.Meta.Official.IsLatest {
		t.Fatalf("prompt meta = %#v, want latest metadata", prompt.Meta.Official)
	}
}

func TestAssetResponseFromPromptResponse(t *testing.T) {
	asset, err := AssetResponseFromPromptResponse(&PromptResponse{
		Prompt: PromptJSON{
			Name:        "welcome-prompt",
			Description: "Welcome system prompt",
			Version:     "1.0.0",
			Content:     "You are helpful.",
		},
		Meta: PromptResponseMeta{Official: &PromptRegistryExtensions{Status: "active", IsLatest: true}},
	})
	if err != nil {
		t.Fatalf("AssetResponseFromPromptResponse() error = %v", err)
	}
	if asset.Asset.ID != "welcome-prompt" {
		t.Fatalf("asset id = %q, want %q", asset.Asset.ID, "welcome-prompt")
	}
	if asset.Asset.Manifest.Entry.Kind != "skill-body" {
		t.Fatalf("entry kind = %q, want %q", asset.Asset.Manifest.Entry.Kind, "skill-body")
	}
	if asset.Asset.SourceSkill.Body != "You are helpful." {
		t.Fatalf("asset content = %q, want %q", asset.Asset.SourceSkill.Body, "You are helpful.")
	}
	if asset.Meta.Official == nil || !asset.Meta.Official.IsLatest {
		t.Fatalf("asset meta = %#v, want latest metadata", asset.Meta.Official)
	}
}

func TestAgentResponseFromAssetResponse(t *testing.T) {
	agent, err := AgentResponseFromAssetResponse(&AssetResponse{
		Asset: Asset{
			ID:          "acme/java-analyzer",
			Name:        "java-analyzer",
			Description: "Analyze Java services",
			Version:     "1.2.0",
			Category:    AssetCategoryAgent,
			SourceSkill: AssetSourceSkill{Path: SkillFileName, Body: "# Java Analyzer\n\nAnalyze Java services", BodyFormat: "markdown"},
			Manifest: AssetManifest{
				SchemaVersion: ShubAssetSchemaVersion,
				ID:            "acme/java-analyzer",
				Category:      AssetCategoryAgent,
				Name:          "java-analyzer",
				Description:   "Analyze Java services",
				Version:       "1.2.0",
				SourceSkill:   AssetSourceSkill{Path: SkillFileName, Body: "# Java Analyzer\n\nAnalyze Java services", BodyFormat: "markdown"},
				Entry:         AssetEntry{Kind: "command", Path: "bin/main.py"},
				Runtime:       AssetRuntime{Type: "python"},
				Metadata: map[string]any{
					assetLegacyAgentMetadataKey: assetLegacyAgentMetadata{
						Agent: AgentJSON{
							AgentManifest: AgentManifest{
								Name:          "java-analyzer",
								Language:      "python",
								Framework:     "adk",
								ModelProvider: "openai",
								ModelName:     "gpt-4o",
								Description:   "Analyze Java services",
							},
							Title:   "Java Analyzer",
							Version: "1.2.0",
						},
					},
				},
			},
			Source: &AssetSource{RepositoryURL: "https://example.com/java-analyzer.git", PackageType: "docker", PackageRef: "ghcr.io/acme/java-analyzer:1.2.0"},
		},
		Meta: AssetResponseMeta{Official: &AssetRegistryExtensions{Status: "active", IsLatest: true}},
	})
	if err != nil {
		t.Fatalf("AgentResponseFromAssetResponse() error = %v", err)
	}
	if agent.Agent.Name != "java-analyzer" {
		t.Fatalf("agent name = %q, want %q", agent.Agent.Name, "java-analyzer")
	}
	if agent.Agent.Title != "Java Analyzer" {
		t.Fatalf("agent title = %q, want %q", agent.Agent.Title, "Java Analyzer")
	}
	if agent.Agent.Language != "python" {
		t.Fatalf("agent language = %q, want %q", agent.Agent.Language, "python")
	}
	if agent.Agent.Repository == nil || agent.Agent.Repository.URL != "https://example.com/java-analyzer.git" {
		t.Fatalf("repository = %#v, want git repository", agent.Agent.Repository)
	}
	if len(agent.Agent.Packages) != 1 || agent.Agent.Packages[0].Identifier != "ghcr.io/acme/java-analyzer:1.2.0" {
		t.Fatalf("packages = %#v, want docker package", agent.Agent.Packages)
	}
	if agent.Meta.Official == nil || !agent.Meta.Official.IsLatest {
		t.Fatalf("agent meta = %#v, want latest metadata", agent.Meta.Official)
	}
}

func TestAssetResponseFromAgentResponse(t *testing.T) {
	asset, err := AssetResponseFromAgentResponse(&AgentResponse{
		Agent: AgentJSON{
			AgentManifest: AgentManifest{
				Name:          "java-analyzer",
				Image:         "ghcr.io/acme/java-analyzer:1.2.0",
				Language:      "python",
				Framework:     "adk",
				ModelProvider: "openai",
				ModelName:     "gpt-4o",
				Description:   "Analyze Java services",
			},
			Title:   "Java Analyzer",
			Version: "1.2.0",
		},
		Meta: AgentResponseMeta{Official: &AgentRegistryExtensions{Status: "active", IsLatest: true}},
	})
	if err != nil {
		t.Fatalf("AssetResponseFromAgentResponse() error = %v", err)
	}
	if asset.Asset.ID != "java-analyzer" {
		t.Fatalf("asset id = %q, want %q", asset.Asset.ID, "java-analyzer")
	}
	if asset.Asset.Category != AssetCategoryAgent {
		t.Fatalf("asset category = %q, want %q", asset.Asset.Category, AssetCategoryAgent)
	}
	if asset.Asset.Manifest.Entry.Kind != "image" {
		t.Fatalf("entry kind = %q, want %q", asset.Asset.Manifest.Entry.Kind, "image")
	}
	if asset.Asset.Manifest.Runtime.Type != "python" {
		t.Fatalf("runtime type = %q, want %q", asset.Asset.Manifest.Runtime.Type, "python")
	}
	if asset.Asset.SourceSkill.Body == "" {
		t.Fatal("asset skill body should not be empty")
	}
	if asset.Meta.Official == nil || !asset.Meta.Official.IsLatest {
		t.Fatalf("asset meta = %#v, want latest metadata", asset.Meta.Official)
	}

	roundTrip, err := AgentResponseFromAssetResponse(asset)
	if err != nil {
		t.Fatalf("round-trip AgentResponseFromAssetResponse() error = %v", err)
	}
	if roundTrip.Agent.Name != "java-analyzer" {
		t.Fatalf("round-trip name = %q, want %q", roundTrip.Agent.Name, "java-analyzer")
	}
	if roundTrip.Agent.Title != "Java Analyzer" {
		t.Fatalf("round-trip title = %q, want %q", roundTrip.Agent.Title, "Java Analyzer")
	}
	if roundTrip.Agent.Image != "ghcr.io/acme/java-analyzer:1.2.0" {
		t.Fatalf("round-trip image = %q, want %q", roundTrip.Agent.Image, "ghcr.io/acme/java-analyzer:1.2.0")
	}
}

func TestAssetPublishRequestToServerJSON(t *testing.T) {
	request := AssetPublishRequest{
		Manifest: AssetManifest{
			SchemaVersion: ShubAssetSchemaVersion,
			ID:            "com.example/weather",
			Category:      AssetCategoryMCP,
			Name:          "Weather API",
			Description:   "Provides weather forecasts",
			Version:       "1.0.0",
			SourceSkill:   AssetSourceSkill{Path: SkillFileName, Body: "# Weather API\n\nREADME", BodyFormat: "markdown"},
			Entry:         AssetEntry{Kind: "mcp-config", Path: "https://api.example.com/mcp"},
			Runtime:       AssetRuntime{Type: "remote"},
		},
	}

	serverJSON, readme, err := request.ToServerJSON()
	if err != nil {
		t.Fatalf("ToServerJSON() error = %v", err)
	}
	if serverJSON.Name != "com.example/weather" {
		t.Fatalf("server name = %q, want %q", serverJSON.Name, "com.example/weather")
	}
	if len(serverJSON.Remotes) != 1 || serverJSON.Remotes[0].URL != "https://api.example.com/mcp" {
		t.Fatalf("remotes = %#v, want mirrored remote", serverJSON.Remotes)
	}
	if readme != "# Weather API\n\nREADME" {
		t.Fatalf("readme = %q, want request README", readme)
	}
}

func TestServerAssetRoundTrip(t *testing.T) {
	asset, err := AssetResponseFromServerResponse(&apiv0.ServerResponse{
		Server: apiv0.ServerJSON{
			Schema:      model.CurrentSchemaURL,
			Name:        "com.example/weather",
			Title:       "Weather API",
			Description: "Provides weather forecasts",
			Version:     "1.2.3",
			Repository:  &model.Repository{URL: "https://github.com/acme/weather", Source: "github"},
			Packages: []model.Package{{
				RegistryType: "npm",
				Identifier:   "@acme/weather-mcp",
				Version:      "1.2.3",
				RunTimeHint:  "npx",
				Transport:    model.Transport{Type: model.TransportTypeStdio},
			}},
			Remotes: []model.Transport{{Type: model.TransportTypeStreamableHTTP, URL: "https://api.example.com/mcp"}},
		},
		Meta: apiv0.ResponseMeta{Official: &apiv0.RegistryExtensions{Status: model.StatusActive, IsLatest: true}},
	}, "# Weather API\n\nREADME")
	if err != nil {
		t.Fatalf("AssetResponseFromServerResponse() error = %v", err)
	}
	if asset.Asset.Category != AssetCategoryMCP {
		t.Fatalf("asset category = %q, want %q", asset.Asset.Category, AssetCategoryMCP)
	}
	if asset.Asset.Name != "Weather API" {
		t.Fatalf("asset name = %q, want %q", asset.Asset.Name, "Weather API")
	}
	if asset.Asset.SourceSkill.Body != "# Weather API\n\nREADME" {
		t.Fatalf("asset readme = %q, want original README", asset.Asset.SourceSkill.Body)
	}

	server, err := ServerResponseFromAssetResponse(asset)
	if err != nil {
		t.Fatalf("ServerResponseFromAssetResponse() error = %v", err)
	}
	if server.Server.Name != "com.example/weather" {
		t.Fatalf("round-trip name = %q, want %q", server.Server.Name, "com.example/weather")
	}
	if server.Server.Title != "Weather API" {
		t.Fatalf("round-trip title = %q, want %q", server.Server.Title, "Weather API")
	}
	if len(server.Server.Packages) != 1 || server.Server.Packages[0].Identifier != "@acme/weather-mcp" {
		t.Fatalf("round-trip packages = %#v, want npm package", server.Server.Packages)
	}
	if len(server.Server.Remotes) != 1 || server.Server.Remotes[0].URL != "https://api.example.com/mcp" {
		t.Fatalf("round-trip remotes = %#v, want remote URL", server.Server.Remotes)
	}
}
