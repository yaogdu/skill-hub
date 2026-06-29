package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpregistry "github.com/agentregistry-dev/agentregistry/internal/mcp/registryserver"
	"github.com/agentregistry-dev/agentregistry/internal/registry/api"
	apitypes "github.com/agentregistry-dev/agentregistry/internal/registry/api/apitypes"
	"github.com/agentregistry-dev/agentregistry/internal/registry/api/router"
	"github.com/agentregistry-dev/agentregistry/internal/registry/config"
	internaldb "github.com/agentregistry-dev/agentregistry/internal/registry/database"
	"github.com/agentregistry-dev/agentregistry/internal/registry/embeddings"
	"github.com/agentregistry-dev/agentregistry/internal/registry/importer"
	"github.com/agentregistry-dev/agentregistry/internal/registry/jobs"
	"github.com/agentregistry-dev/agentregistry/internal/registry/kinds"
	"github.com/agentregistry-dev/agentregistry/internal/registry/seed"
	"github.com/agentregistry-dev/agentregistry/internal/registry/service"
	agentsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/agent"
	assetsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/asset"
	promptsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/prompt"
	serversvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/server"
	shubsourcesvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/shubsource"
	skillsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/skill"
	userauthsvc "github.com/agentregistry-dev/agentregistry/internal/registry/service/userauth"
	"github.com/agentregistry-dev/agentregistry/internal/registry/telemetry"
	"github.com/agentregistry-dev/agentregistry/internal/version"
	"github.com/agentregistry-dev/agentregistry/pkg/logging"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
	"github.com/agentregistry-dev/agentregistry/pkg/types"
)

const defaultDevJWTSeed = "0000000000000000000000000000000000000000000000000000000000000000"

func App(ctx context.Context, opts ...types.AppOptions) error {
	var options types.AppOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	cfg := config.NewConfig()
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create a context with timeout for PostgreSQL connection
	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	setupLogging(cfg.LogLevel)

	// Build auth providers from options (before database creation). The built-in
	// username/password login and API key flow require a JWT signer; when the
	// operator does not provide one, we fall back to a deterministic development
	// seed so local/private deployments still boot with auth enabled.
	if cfg.JWTPrivateKey == "" {
		cfg.JWTPrivateKey = defaultDevJWTSeed
		slog.Warn("AGENT_REGISTRY_JWT_PRIVATE_KEY is not set; using the built-in development signing key")
	}
	jwtManager := auth.NewJWTManager(cfg)

	authnProvider := options.AuthnProvider
	defaultAuthnProvider := auth.NewRegistryAuthnProvider(jwtManager)
	if authnProvider == nil {
		authnProvider = defaultAuthnProvider
	}

	// Resolve authz provider: use provided, or default to ownership-aware authz.
	authzProvider := options.AuthzProvider
	defaultAuthzProvider := &auth.RegistryAuthzProvider{}
	if authzProvider == nil {
		publicActions, err := auth.ResolvePublicActions(cfg.PublicActions, true)
		if err != nil {
			return fmt.Errorf("invalid AGENT_REGISTRY_PUBLIC_ACTIONS: %w", err)
		}
		slog.Info("using registry authz provider", "jwt_enabled", true, "public_actions", publicActions)
		defaultAuthzProvider = auth.NewRegistryAuthzProvider(publicActions)
		authzProvider = defaultAuthzProvider
	}
	authz := auth.Authorizer{Authz: authzProvider}

	// Database selection: use DATABASE_URL="noop" only when you provide the database
	// entirely via AppOptions.DatabaseFactory (e.g. in-memory or custom backend) and
	// do not want a real PostgreSQL connection. In that case DatabaseFactory is required.
	// For normal deployments, set DATABASE_URL to a real Postgres connection string.
	var db database.Store
	if cfg.DatabaseURL == "noop" { //nolint:nestif
		if options.DatabaseFactory == nil {
			return fmt.Errorf("DATABASE_URL=noop requires DatabaseFactory to be set in AppOptions")
		}
		slog.Info("using DatabaseFactory to create database", "mode", "noop")
		var err error
		db, err = options.DatabaseFactory(ctx, "", nil, authz)
		if err != nil {
			return fmt.Errorf("failed to create database via factory: %w", err)
		}
	} else {
		baseDB, err := internaldb.NewPostgreSQL(dbCtx, cfg.DatabaseURL, authz, cfg.DatabaseVectorEnabled)
		if err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}

		// Allow implementors to wrap the database and run additional migrations
		db = baseDB
		if options.DatabaseFactory != nil {
			db, err = options.DatabaseFactory(ctx, cfg.DatabaseURL, baseDB, authz)
			if err != nil {
				if err := baseDB.Close(); err != nil {
					slog.Error("error closing base database connection", "error", err)
				}
				return fmt.Errorf("failed to create extended database: %w", err)
			}
		}
	}

	// Store the database instance for later cleanup
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("error closing database connection", "error", err)
		} else {
			slog.Info("database connection closed successfully")
		}
	}()

	var registryUserStore internaldb.RegistryUserStore
	var registryAPIKeyStore internaldb.RegistryAPIKeyStore
	var resourceOwnerStore internaldb.ResourceOwnerStore
	var authSettingsStore internaldb.RegistryAuthSettingsStore
	if provider, ok := db.(interface {
		RegistryUsers() internaldb.RegistryUserStore
		RegistryAPIKeys() internaldb.RegistryAPIKeyStore
		ResourceOwners() internaldb.ResourceOwnerStore
		RegistryAuthSettings() internaldb.RegistryAuthSettingsStore
	}); ok {
		registryUserStore = provider.RegistryUsers()
		registryAPIKeyStore = provider.RegistryAPIKeys()
		resourceOwnerStore = provider.ResourceOwners()
		authSettingsStore = provider.RegistryAuthSettings()
	}
	if defaultAuthnProvider != nil {
		defaultAuthnProvider.SetStores(registryUserStore, registryAPIKeyStore)
	}
	if defaultAuthzProvider != nil {
		defaultAuthzProvider.SetOwnerLookup(resourceOwnerStore)
		defaultAuthzProvider.SetAuthSettingsLookup(authSettingsStore)
	}

	var embeddingProvider embeddings.Provider
	if cfg.Embeddings.Enabled {
		client := &http.Client{Timeout: 30 * time.Second}
		if provider, err := embeddings.Factory(&cfg.Embeddings, client); err != nil {
			slog.Warn("semantic embeddings disabled", "error", err)
		} else {
			embeddingProvider = provider
		}
	}

	serverService := serversvc.New(serversvc.Dependencies{
		Servers:            db.Servers(),
		Tx:                 db,
		Config:             cfg,
		EmbeddingsProvider: embeddingProvider,
	})
	agentService := agentsvc.New(agentsvc.Dependencies{
		Agents:             db.Agents(),
		Skills:             db.Skills(),
		Prompts:            db.Prompts(),
		Tx:                 db,
		Config:             cfg,
		EmbeddingsProvider: embeddingProvider,
	})
	skillService := skillsvc.New(skillsvc.Dependencies{Skills: db.Skills(), Tx: db})
	packageStore, err := assetsvc.NewFilesystemPackageStore(cfg.StorageDir, authz)
	if err != nil {
		return fmt.Errorf("initialize asset package store: %w", err)
	}
	assetService := assetsvc.New(assetsvc.Dependencies{StoreDB: db, Skills: skillService, Packages: packageStore, Tx: db})
	var shubSourceStore database.SHUBSourceStore
	if provider, ok := db.(interface {
		Sources() database.SHUBSourceStore
	}); ok {
		shubSourceStore = provider.Sources()
	}
	var shubSourceService shubsourcesvc.Registry
	if shubSourceStore != nil {
		shubSourceService = shubsourcesvc.New(shubsourcesvc.Dependencies{Sources: shubSourceStore, Assets: assetService})
	}
	userAuthService := userauthsvc.New(registryUserStore, registryAPIKeyStore, authSettingsStore, jwtManager)
	if err := userAuthService.BootstrapAdmin(auth.WithSystemContext(context.Background()), cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		return fmt.Errorf("bootstrap registry admin: %w", err)
	}
	promptService := promptsvc.New(promptsvc.Dependencies{Prompts: db.Prompts(), Tx: db})
	// Import builtin seed data unless it is disabled
	if !cfg.DisableBuiltinSeed {
		slog.Info("importing builtin seed data in the background")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			ctx = auth.WithSystemContext(ctx)

			if err := seed.ImportBuiltinSeedData(ctx, serverService); err != nil {
				slog.Error("failed to import builtin seed data", "error", err)
			}
		}()
	}

	// Import seed data if seed source is provided
	if cfg.SeedFrom != "" {
		slog.Info("importing data in the background", "seed_from", cfg.SeedFrom)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			ctx = auth.WithSystemContext(ctx)

			importerService := importer.NewService(serverService)
			if embeddingProvider != nil {
				importerService.SetEmbeddingProvider(embeddingProvider)
				importerService.SetEmbeddingDimensions(cfg.Embeddings.Dimensions)
				importerService.SetGenerateEmbeddings(cfg.Embeddings.Enabled)
			}
			if err := importerService.ImportFromPath(ctx, cfg.SeedFrom, cfg.EnrichServerData); err != nil {
				slog.Error("failed to import seed data", "error", err)
			}
		}()
	}

	slog.Info("starting agentregistry", "version", version.Version, "commit", version.GitCommit)

	// Prepare version information
	versionInfo := &apitypes.VersionBody{
		Version:   version.Version,
		GitCommit: version.GitCommit,
		BuildTime: version.BuildDate,
	}

	shutdownTelemetry, metrics, err := telemetry.InitMetrics(cfg.Version)
	if err != nil {
		return fmt.Errorf("failed to initialize metrics: %v", err)
	}

	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			slog.Error("failed to shutdown telemetry", "error", err)
		}
	}()

	// Build the kind registry for asset management APIs.
	kindReg := kinds.NewRegistry()
	kindReg.Register(kinds.Kind{
		Kind:     "agent",
		Plural:   "agents",
		Aliases:  []string{"Agent"},
		SpecType: reflect.TypeFor[kinds.AgentSpec](),
		Apply:    kinds.MakeApplyFunc("agent", kinds.ToAgentJSON, agentService.ApplyAgent, agentService.GetAgentVersion),
		Get:      kinds.MakeGetFunc(agentService.GetAgent, agentService.GetAgentVersion),
		Delete:   agentService.DeleteAgent,
		TableColumns: []kinds.Column{
			{Header: "NAME"}, {Header: "VERSION"}, {Header: "FRAMEWORK"},
			{Header: "LANGUAGE"}, {Header: "PROVIDER"}, {Header: "MODEL"},
		},
		InitTemplate: kinds.MakeInitTemplate("agent", kinds.AgentSpec{Description: "TODO: describe your agent"}),
	})
	kindReg.Register(kinds.Kind{
		Kind:     "skill",
		Plural:   "skills",
		Aliases:  []string{"Skill"},
		SpecType: reflect.TypeFor[kinds.SkillSpec](),
		Apply:    kinds.MakeApplyFunc("skill", kinds.ToSkillJSON, skillService.ApplySkill, skillService.GetSkillVersion),
		Get:      kinds.MakeGetFunc(skillService.GetSkill, skillService.GetSkillVersion),
		Delete:   skillService.DeleteSkill,
		TableColumns: []kinds.Column{
			{Header: "NAME"}, {Header: "VERSION"}, {Header: "CATEGORY"}, {Header: "DESCRIPTION"},
		},
		InitTemplate: kinds.MakeInitTemplate("skill", kinds.SkillSpec{Description: "TODO: describe your skill"}),
	})
	kindReg.Register(kinds.Kind{
		Kind:     "prompt",
		Plural:   "prompts",
		Aliases:  []string{"Prompt"},
		SpecType: reflect.TypeFor[kinds.PromptSpec](),
		Apply:    kinds.MakeApplyFunc("prompt", kinds.ToPromptJSON, promptService.ApplyPrompt, promptService.GetPromptVersion),
		Get:      kinds.MakeGetFunc(promptService.GetPrompt, promptService.GetPromptVersion),
		Delete:   promptService.DeletePrompt,
		TableColumns: []kinds.Column{
			{Header: "NAME"}, {Header: "VERSION"}, {Header: "DESCRIPTION"},
		},
		InitTemplate: kinds.MakeInitTemplate("prompt", kinds.PromptSpec{Description: "TODO: describe your prompt", Content: "TODO: write your prompt content"}),
	})
	kindReg.Register(kinds.Kind{
		Kind:     "mcp",
		Plural:   "mcps",
		Aliases:  []string{"MCPServer", "mcpserver", "mcp-server", "mcpservers"},
		SpecType: reflect.TypeFor[kinds.MCPSpec](),
		Apply:    kinds.MakeApplyFunc("mcp", kinds.ToServerJSON, serverService.ApplyServer, serverService.GetServerVersion),
		Get:      kinds.MakeGetFunc(serverService.GetServer, serverService.GetServerVersion),
		Delete:   serverService.DeleteServer,
		TableColumns: []kinds.Column{
			{Header: "NAME"}, {Header: "VERSION"}, {Header: "DESCRIPTION"},
		},
		InitTemplate: kinds.MakeInitTemplate("mcp", kinds.MCPSpec{Description: "TODO: describe your MCP server"}),
	})
	routeOpts := &router.RouteOptions{
		ExtraRoutes:  options.ExtraRoutes,
		KindRegistry: kindReg,
	}

	// Initialize job manager and indexer for embeddings.
	if cfg.Embeddings.Enabled && embeddingProvider != nil {
		jobManager := jobs.NewManager()
		indexer := service.NewIndexer(serverService, agentService, embeddingProvider, cfg.Embeddings.Dimensions)
		routeOpts.Indexer = indexer
		routeOpts.JobManager = jobManager
		slog.Info("embeddings indexing API enabled")
	}

	// Initialize HTTP server
	baseServer := api.NewServer(cfg, router.RegistryServices{
		Server:     serverService,
		Agent:      agentService,
		Skill:      skillService,
		Asset:      assetService,
		Prompt:     promptService,
		SHUBSource: shubSourceService,
		UserAuth:   userAuthService,
	}, metrics, versionInfo, options.UIHandler, authnProvider, routeOpts)

	var server types.Server
	if options.HTTPServerFactory != nil {
		server = options.HTTPServerFactory(baseServer, db)
	} else {
		server = baseServer
	}

	if options.OnHTTPServerCreated != nil {
		options.OnHTTPServerCreated(server)
	}

	var mcpHTTPServer *http.Server
	if cfg.MCPPort > 0 {
		mcpServer := mcpregistry.NewServer(serverService, agentService, skillService)

		var handler http.Handler = mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return mcpServer
		}, &mcp.StreamableHTTPOptions{})

		// Set up authentication middleware if one is configured
		if authnProvider != nil {
			handler = mcpAuthnMiddleware(authnProvider)(handler)
		}

		addr := ":" + strconv.Itoa(int(cfg.MCPPort))
		mcpHTTPServer = &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}

		go func() {
			slog.Info("MCP HTTP server starting", "address", addr)
			if err := mcpHTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("failed to start MCP server", "error", err)
				os.Exit(1)
			}
		}()
	}

	// Start server in a goroutine so it doesn't block signal handling
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	// Create context with timeout for shutdown
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()

	// Gracefully shutdown the server
	if err := server.Shutdown(sctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
	if mcpHTTPServer != nil {
		if err := mcpHTTPServer.Shutdown(sctx); err != nil {
			slog.Error("MCP server forced to shutdown", "error", err)
		}
	}

	slog.Info("server exiting")
	return nil
}

// mcpAuthnMiddleware creates a middleware that uses the AuthnProvider to authenticate requests and add to session context.
// this session context is used by the db + authz provider to check permissions.
func mcpAuthnMiddleware(authn auth.AuthnProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// authenticate using the configured provider
			session, err := authn.Authenticate(ctx, r.Header.Get, r.URL.Query())
			if err == nil && session != nil {
				ctx = auth.AuthSessionTo(ctx, session)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// setupLogging configures the global slog logger
func setupLogging(levelStr string) {
	logging.SetupDefault()

	if levelStr == "" {
		levelStr = "info"
	}
	level, err := logging.ParseLevel(levelStr)
	if err != nil {
		slog.Error("failed to parse log level, defaulting to info", "error", err)
		level = slog.LevelInfo
	}
	// set all loggers to the specified level
	logging.Reset(level)
}
