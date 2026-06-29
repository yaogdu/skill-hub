package types

import (
	"context"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
)

// ProviderAdapter defines legacy provider CRUD behavior for an internal
// provider platform type.
type ProviderAdapter interface {
	Platform() string
	ListProviders(ctx context.Context) ([]*models.Provider, error)
	CreateProvider(ctx context.Context, in *models.CreateProviderInput) (*models.Provider, error)
	GetProvider(ctx context.Context, providerID string) (*models.Provider, error)
	UpdateProvider(ctx context.Context, providerID string, in *models.UpdateProviderInput) (*models.Provider, error)
	DeleteProvider(ctx context.Context, providerID string) error
}

// DeploymentAdapter defines legacy deployment behavior for an internal provider
// platform type.
type DeploymentAdapter interface {
	Platform() string
	SupportedResourceTypes() []string
	Deploy(ctx context.Context, req *models.Deployment) (*models.DeploymentActionResult, error)
	Undeploy(ctx context.Context, deployment *models.Deployment) error
	GetLogs(ctx context.Context, deployment *models.Deployment) ([]string, error)
	Cancel(ctx context.Context, deployment *models.Deployment) error
	Discover(ctx context.Context, providerID string) ([]*models.Deployment, error)
}
