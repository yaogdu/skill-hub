package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/agentregistry-dev/agentregistry/pkg/models"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

type shubSourceStore struct {
	repositoryBase
}

var _ database.SHUBSourceStore = (*shubSourceStore)(nil)

func (s *shubSourceStore) ListSHUBSources(ctx context.Context) ([]*models.SHUBSource, error) {
	if err := s.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Type: auth.PermissionArtifactTypeSource}); err != nil {
		return nil, err
	}
	rows, err := s.executor.Query(ctx, `SELECT name, address, created_at, updated_at FROM shub_sources ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list SHUB sources: %w", err)
	}
	defer rows.Close()

	result := make([]*models.SHUBSource, 0)
	for rows.Next() {
		var source models.SHUBSource
		if err := rows.Scan(&source.Name, &source.Address, &source.CreatedAt, &source.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan SHUB source: %w", err)
		}
		result = append(result, &source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SHUB sources: %w", err)
	}
	return result, nil
}

func (s *shubSourceStore) GetSHUBSource(ctx context.Context, name string) (*models.SHUBSource, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, database.ErrInvalidInput
	}
	if err := s.authz.Check(ctx, auth.PermissionActionRead, auth.Resource{Name: trimmedName, Type: auth.PermissionArtifactTypeSource}); err != nil {
		return nil, err
	}

	var source models.SHUBSource
	if err := s.executor.QueryRow(ctx, `SELECT name, address, created_at, updated_at FROM shub_sources WHERE name = $1`, trimmedName).Scan(
		&source.Name,
		&source.Address,
		&source.CreatedAt,
		&source.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, database.ErrNotFound
		}
		return nil, fmt.Errorf("get SHUB source %q: %w", trimmedName, err)
	}
	return &source, nil
}

func (s *shubSourceStore) PutSHUBSource(ctx context.Context, source *models.SHUBSource) (*models.SHUBSource, error) {
	if source == nil {
		return nil, database.ErrInvalidInput
	}
	name := strings.TrimSpace(source.Name)
	address := strings.TrimSpace(source.Address)
	if name == "" || address == "" {
		return nil, database.ErrInvalidInput
	}

	var exists bool
	if err := s.executor.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shub_sources WHERE name = $1)`, name).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check SHUB source existence %q: %w", name, err)
	}
	action := auth.PermissionActionPublish
	if exists {
		action = auth.PermissionActionEdit
	}
	if err := s.authz.Check(ctx, action, auth.Resource{Name: name, Type: auth.PermissionArtifactTypeSource}); err != nil {
		return nil, err
	}

	stored := &models.SHUBSource{}
	if err := s.executor.QueryRow(ctx, `
		INSERT INTO shub_sources (name, address)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET address = EXCLUDED.address,
		    updated_at = NOW()
		RETURNING name, address, created_at, updated_at
	`, name, address).Scan(&stored.Name, &stored.Address, &stored.CreatedAt, &stored.UpdatedAt); err != nil {
		return nil, fmt.Errorf("put SHUB source %q: %w", name, err)
	}
	return stored, nil
}

func (s *shubSourceStore) DeleteSHUBSource(ctx context.Context, name string) error {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return database.ErrInvalidInput
	}
	if err := s.authz.Check(ctx, auth.PermissionActionDelete, auth.Resource{Name: trimmedName, Type: auth.PermissionArtifactTypeSource}); err != nil {
		return err
	}
	result, err := s.executor.Exec(ctx, `DELETE FROM shub_sources WHERE name = $1`, trimmedName)
	if err != nil {
		return fmt.Errorf("delete SHUB source %q: %w", trimmedName, err)
	}
	if result.RowsAffected() == 0 {
		return database.ErrNotFound
	}
	return nil
}
