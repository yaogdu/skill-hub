package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

// PostgreSQL is the root PostgreSQL-backed store. It owns the pool, authz, and
// transaction orchestration, while domain-specific repository structs own CRUD.
type PostgreSQL struct {
	pool      *pgxpool.Pool
	authz     auth.Authorizer
	rootScope *postgresScope
}

type repositoryBase struct {
	executor executor
	authz    auth.Authorizer
	owners   *resourceOwnerStore
}

type postgresScope struct {
	users        *registryUserStore
	apiKeys      *registryAPIKeyStore
	owners       *resourceOwnerStore
	authSettings *registryAuthSettingsStore
	servers      *serverStore
	providers    *providerStore
	sources      *shubSourceStore
	agents       *agentStore
	skills       *skillStore
	assets       *assetStore
	prompts      *promptStore
	deployments  *deploymentStore
}

var _ database.Scope = (*postgresScope)(nil)

// executor is the internal query surface used by repository methods.
// Both *pgxpool.Pool and pgx.Tx satisfy this interface natively.
type executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func newPostgresScope(executor executor, authz auth.Authorizer, tx pgx.Tx) *postgresScope {
	ownerBase := repositoryBase{executor: executor, authz: authz}
	owners := &resourceOwnerStore{repositoryBase: ownerBase}
	base := repositoryBase{executor: executor, authz: authz, owners: owners}
	return &postgresScope{
		users:        &registryUserStore{repositoryBase: ownerBase},
		apiKeys:      &registryAPIKeyStore{repositoryBase: ownerBase},
		owners:       owners,
		authSettings: &registryAuthSettingsStore{repositoryBase: ownerBase},
		servers:      &serverStore{repositoryBase: base, tx: tx},
		providers:    &providerStore{repositoryBase: base},
		sources:      &shubSourceStore{repositoryBase: base},
		agents:       &agentStore{repositoryBase: base},
		skills:       &skillStore{repositoryBase: base},
		assets:       &assetStore{repositoryBase: base},
		prompts:      &promptStore{repositoryBase: base},
		deployments:  &deploymentStore{repositoryBase: base, tx: tx},
	}
}

func (s *postgresScope) Servers() database.ServerStore {
	return s.servers
}

func (s *postgresScope) Providers() database.ProviderStore {
	return s.providers
}

func (s *postgresScope) RegistryUsers() RegistryUserStore {
	return s.users
}

func (s *postgresScope) RegistryAPIKeys() RegistryAPIKeyStore {
	return s.apiKeys
}

func (s *postgresScope) ResourceOwners() ResourceOwnerStore {
	return s.owners
}

func (s *postgresScope) RegistryAuthSettings() RegistryAuthSettingsStore {
	return s.authSettings
}

func (s *postgresScope) Sources() database.SHUBSourceStore {
	return s.sources
}

func (s *postgresScope) Agents() database.AgentStore {
	return s.agents
}

func (s *postgresScope) Skills() database.SkillStore {
	return s.skills
}

func (s *postgresScope) Assets() database.AssetStore {
	return s.assets
}

func (s *postgresScope) Prompts() database.PromptStore {
	return s.prompts
}

func (s *postgresScope) Deployments() database.DeploymentStore {
	return s.deployments
}

func NewPostgreSQL(ctx context.Context, connectionURI string, authz auth.Authorizer, vectorEnabled bool) (*PostgreSQL, error) {
	// Parse connection config for pool settings
	config, err := pgxpool.ParseConfig(connectionURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PostgreSQL config: %w", err)
	}

	// Configure pool for stability-focused defaults
	config.MaxConns = 30                      // Handle good concurrent load
	config.MinConns = 5                       // Keep connections warm for fast response
	config.MaxConnIdleTime = 30 * time.Minute // Keep connections available for bursts
	config.MaxConnLifetime = 2 * time.Hour    // Refresh connections regularly for stability

	// Create connection pool with configured settings
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create PostgreSQL pool: %w", err)
	}

	// Test the connection
	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Run migrations using a single connection from the pool
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection for migrations: %w", err)
	}
	defer conn.Release()

	migrator := database.NewMigrator(conn.Conn(), DefaultMigratorConfig())
	if err := migrator.Migrate(ctx); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	if vectorEnabled {
		vectorMigrator := database.NewMigrator(conn.Conn(), VectorMigratorConfig())
		if err := vectorMigrator.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("failed to run vector database migrations: %w", err)
		}
	}

	db := &PostgreSQL{pool: pool, authz: authz}
	db.rootScope = newPostgresScope(pool, authz, nil)
	return db, nil
}

func (db *PostgreSQL) Servers() database.ServerStore     { return db.rootScope.servers }
func (db *PostgreSQL) Providers() database.ProviderStore { return db.rootScope.providers }
func (db *PostgreSQL) Sources() database.SHUBSourceStore { return db.rootScope.sources }
func (db *PostgreSQL) RegistryUsers() RegistryUserStore  { return db.rootScope.users }
func (db *PostgreSQL) RegistryAPIKeys() RegistryAPIKeyStore {
	return db.rootScope.apiKeys
}
func (db *PostgreSQL) ResourceOwners() ResourceOwnerStore { return db.rootScope.owners }
func (db *PostgreSQL) RegistryAuthSettings() RegistryAuthSettingsStore {
	return db.rootScope.authSettings
}
func (db *PostgreSQL) Agents() database.AgentStore   { return db.rootScope.agents }
func (db *PostgreSQL) Skills() database.SkillStore   { return db.rootScope.skills }
func (db *PostgreSQL) Assets() database.AssetStore   { return db.rootScope.assets }
func (db *PostgreSQL) Prompts() database.PromptStore { return db.rootScope.prompts }
func (db *PostgreSQL) Deployments() database.DeploymentStore {
	return db.rootScope.deployments
}

func (db *PostgreSQL) InTransaction(ctx context.Context, fn func(ctx context.Context, scope database.Scope) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	//nolint:contextcheck // Intentionally using separate context for rollback to ensure cleanup even if request is cancelled
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if rbErr := tx.Rollback(rollbackCtx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			slog.Error("failed to rollback transaction", "error", rbErr)
		}
	}()

	txScope := newPostgresScope(tx, db.authz, tx)
	if err := fn(ctx, txScope); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (db *PostgreSQL) Close() error {
	db.pool.Close()
	return nil
}
