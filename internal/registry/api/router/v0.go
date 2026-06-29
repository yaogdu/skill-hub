// Package router contains API routing logic
package router

import (
	"net/http"

	apitypes "github.com/agentregistry-dev/agentregistry/internal/registry/api/apitypes"
	v0agents "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/agents"
	v0apply "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/apply"
	v0assets "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/assets"
	v0embeddings "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/embeddings"
	v0health "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/health"
	v0ping "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/ping"
	v0prompts "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/prompts"
	v0servers "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/servers"
	v0shubsources "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/shubsources"
	v0skills "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/skills"
	v0userauth "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/userauth"
	v0version "github.com/agentregistry-dev/agentregistry/internal/registry/api/handlers/v0/version"
	"github.com/agentregistry-dev/agentregistry/internal/registry/kinds"
	agentsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/agent"
	assetsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/asset"
	promptsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/prompt"
	serversvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/server"
	shubsourcesvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/shubsource"
	skillsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/skill"
	userauthsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/userauth"
	"github.com/danielgtaylor/huma/v2"

	"github.com/agentregistry-dev/agentregistry/internal/registry/config"
	"github.com/agentregistry-dev/agentregistry/internal/registry/jobs"
	"github.com/agentregistry-dev/agentregistry/internal/registry/service"
	"github.com/agentregistry-dev/agentregistry/internal/registry/telemetry"
)

// RegistryServices bundles all per-domain service registries for route registration.
type RegistryServices struct {
	Server     serversvc.Registry
	Agent      agentsvc.Registry
	Skill      skillsvc.Registry
	Asset      assetsvc.Registry
	Prompt     promptsvc.Registry
	SHUBSource shubsourcesvc.Registry
	UserAuth   userauthsvc.Registry
}

// RouteOptions contains optional services for route registration.
type RouteOptions struct {
	Indexer    service.Indexer
	JobManager *jobs.Manager
	Mux        *http.ServeMux

	// Optional callback for integration-owned route registration.
	ExtraRoutes func(api huma.API, pathPrefix string)

	// KindRegistry is the declarative kind registry used by POST/DELETE /v0/apply.
	// When non-nil the batch apply endpoints are registered.
	KindRegistry *kinds.Registry
}

// RegisterRoutes registers all API routes under /v0.
func RegisterRoutes(
	api huma.API,
	cfg *config.Config,
	svcs RegistryServices,
	metrics *telemetry.Metrics,
	versionInfo *apitypes.VersionBody,
	opts *RouteOptions,
) {
	pathPrefix := "/v0"

	v0health.RegisterHealthEndpoint(api, pathPrefix, cfg, metrics)
	v0ping.RegisterPingEndpoint(api, pathPrefix)
	v0version.RegisterVersionEndpoint(api, pathPrefix, versionInfo)
	if svcs.UserAuth != nil {
		v0userauth.RegisterAuthEndpoints(api, pathPrefix, svcs.UserAuth)
	}
	v0servers.RegisterServersEndpoints(api, pathPrefix, svcs.Server)
	v0servers.RegisterServersCreateEndpoint(api, pathPrefix, svcs.Server)
	v0servers.RegisterEditEndpoints(api, pathPrefix, svcs.Server)
	if svcs.SHUBSource != nil {
		v0shubsources.RegisterSHUBSourceEndpoints(api, pathPrefix, svcs.SHUBSource)
	}
	v0agents.RegisterAgentsEndpoints(api, pathPrefix, svcs.Agent)
	v0agents.RegisterAgentsCreateEndpoint(api, pathPrefix, svcs.Agent)
	v0assets.RegisterAssetsEndpoints(api, pathPrefix, svcs.Asset)
	v0assets.RegisterAssetsCreateEndpoint(api, pathPrefix, svcs.Asset)
	v0skills.RegisterSkillsEndpoints(api, pathPrefix, svcs.Skill)
	v0skills.RegisterSkillsCreateEndpoint(api, pathPrefix, svcs.Skill)
	v0prompts.RegisterPromptsEndpoints(api, pathPrefix, svcs.Prompt)
	v0prompts.RegisterPromptsCreateEndpoint(api, pathPrefix, svcs.Prompt)

	if opts != nil && opts.Indexer != nil && opts.JobManager != nil {
		v0embeddings.RegisterEmbeddingsEndpoints(api, pathPrefix, opts.Indexer, opts.JobManager)
		if opts.Mux != nil {
			v0embeddings.RegisterEmbeddingsSSEHandler(opts.Mux, pathPrefix, opts.Indexer, opts.JobManager)
		}
	}
	if opts != nil && opts.KindRegistry != nil {
		v0apply.RegisterApplyEndpoints(api, pathPrefix, opts.KindRegistry)
	}
	if opts != nil && opts.ExtraRoutes != nil {
		opts.ExtraRoutes(api, pathPrefix)
	}
}
