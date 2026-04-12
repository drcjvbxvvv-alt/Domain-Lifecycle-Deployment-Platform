package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// HostGroup maps to the host_groups table.
type HostGroup struct {
	ID                   int64      `db:"id"`
	UUID                 string     `db:"uuid"`
	ProjectID            int64      `db:"project_id"`
	Name                 string     `db:"name"`
	Description          *string    `db:"description"`
	Region               *string    `db:"region"`
	MaxConcurrency       int        `db:"max_concurrency"`       // 0 = unlimited
	ReloadBatchSize      int        `db:"reload_batch_size"`     // domains per nginx reload batch
	ReloadBatchWaitSecs  int        `db:"reload_batch_wait_secs"` // seconds to buffer before reload
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
	DeletedAt            *time.Time `db:"deleted_at"`
}

// HostGroupStore handles persistence for host_groups.
type HostGroupStore struct {
	db *sqlx.DB
}

// NewHostGroupStore creates a HostGroupStore.
func NewHostGroupStore(db *sqlx.DB) *HostGroupStore {
	return &HostGroupStore{db: db}
}

// GetByID returns a host group by its numeric ID.
func (s *HostGroupStore) GetByID(ctx context.Context, id int64) (*HostGroup, error) {
	var hg HostGroup
	err := s.db.GetContext(ctx, &hg,
		`SELECT id, uuid, project_id, name, description, region,
		        max_concurrency, reload_batch_size, reload_batch_wait_secs,
		        created_at, updated_at, deleted_at
		 FROM host_groups WHERE id = $1 AND deleted_at IS NULL`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get host group %d: %w", id, err)
	}
	return &hg, nil
}

// List returns all non-deleted host groups.
func (s *HostGroupStore) List(ctx context.Context) ([]HostGroup, error) {
	var items []HostGroup
	err := s.db.SelectContext(ctx, &items,
		`SELECT id, uuid, project_id, name, description, region,
		        max_concurrency, reload_batch_size, reload_batch_wait_secs,
		        created_at, updated_at, deleted_at
		 FROM host_groups WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list host groups: %w", err)
	}
	return items, nil
}

// ListByProject returns all non-deleted host groups for a project.
func (s *HostGroupStore) ListByProject(ctx context.Context, projectID int64) ([]HostGroup, error) {
	var items []HostGroup
	err := s.db.SelectContext(ctx, &items,
		`SELECT id, uuid, project_id, name, description, region,
		        max_concurrency, reload_batch_size, reload_batch_wait_secs,
		        created_at, updated_at, deleted_at
		 FROM host_groups WHERE project_id = $1 AND deleted_at IS NULL ORDER BY id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list host groups by project %d: %w", projectID, err)
	}
	return items, nil
}

// UpdateConcurrency updates the concurrency and batching settings for a host group.
func (s *HostGroupStore) UpdateConcurrency(ctx context.Context, id int64, maxConcurrency, reloadBatchSize, reloadBatchWaitSecs int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE host_groups
		 SET max_concurrency = $1, reload_batch_size = $2, reload_batch_wait_secs = $3, updated_at = NOW()
		 WHERE id = $4 AND deleted_at IS NULL`,
		maxConcurrency, reloadBatchSize, reloadBatchWaitSecs, id)
	if err != nil {
		return fmt.Errorf("update host group concurrency %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("host group %d not found", id)
	}
	return nil
}

// CountInFlight returns the number of agent_tasks currently in claimed or running
// state for agents that belong to the given host group.
func (s *HostGroupStore) CountInFlight(ctx context.Context, hostGroupID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM agent_tasks at
		 JOIN agents a ON a.id = at.agent_id
		 WHERE a.host_group_id = $1
		 AND at.status IN ('claimed', 'running')`, hostGroupID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count in-flight tasks for host group %d: %w", hostGroupID, err)
	}
	return count, nil
}
