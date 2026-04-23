package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	ErrProbeNodeNotFound       = errors.New("probe node not found")
	ErrCheckAssignmentNotFound = errors.New("check assignment not found")
)

// ── Models ────────────────────────────────────────────────────────────────────

// GFWProbeNode maps to gfw_probe_nodes.
type GFWProbeNode struct {
	ID           int64           `db:"id"`
	UUID         string          `db:"uuid"`
	NodeID       string          `db:"node_id"`
	Region       string          `db:"region"`
	Role         string          `db:"role"`
	Status       string          `db:"status"`
	LastSeenAt   *time.Time      `db:"last_seen_at"`
	AgentVersion *string         `db:"agent_version"`
	IPAddress    *string         `db:"ip_address"`
	Metadata     json.RawMessage `db:"metadata"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

// GFWCheckAssignment maps to gfw_check_assignments.
type GFWCheckAssignment struct {
	ID             int64           `db:"id"`
	UUID           string          `db:"uuid"`
	DomainID       int64           `db:"domain_id"`
	ProbeNodeIDs   json.RawMessage `db:"probe_node_ids"`
	ControlNodeIDs json.RawMessage `db:"control_node_ids"`
	CheckInterval  int             `db:"check_interval"`
	Enabled        bool            `db:"enabled"`
	CreatedAt      time.Time       `db:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"`
}

// ── Store ─────────────────────────────────────────────────────────────────────

type GFWNodeStore struct {
	db *sqlx.DB
}

func NewGFWNodeStore(db *sqlx.DB) *GFWNodeStore {
	return &GFWNodeStore{db: db}
}

// ── Probe node writes ─────────────────────────────────────────────────────────

// Register upserts a probe node row and transitions it to "online".
// Idempotent — safe to call on every restart.
func (s *GFWNodeStore) Register(ctx context.Context, node *GFWProbeNode) error {
	now := time.Now()
	return s.db.QueryRowContext(ctx, `
		INSERT INTO gfw_probe_nodes
		  (node_id, region, role, status, last_seen_at, agent_version, ip_address, metadata)
		VALUES ($1, $2, $3, 'online', $4, $5, $6, $7)
		ON CONFLICT (node_id) DO UPDATE SET
		  region        = EXCLUDED.region,
		  role          = EXCLUDED.role,
		  status        = 'online',
		  last_seen_at  = EXCLUDED.last_seen_at,
		  agent_version = EXCLUDED.agent_version,
		  ip_address    = EXCLUDED.ip_address,
		  metadata      = EXCLUDED.metadata,
		  updated_at    = NOW()
		RETURNING id, uuid, created_at, updated_at`,
		node.NodeID, node.Region, node.Role, now,
		node.AgentVersion, node.IPAddress, nullJSON(node.Metadata),
	).Scan(&node.ID, &node.UUID, &node.CreatedAt, &node.UpdatedAt)
}

// Heartbeat updates last_seen_at and marks the node online.
// Returns ErrProbeNodeNotFound if node_id is unknown.
func (s *GFWNodeStore) Heartbeat(ctx context.Context, nodeID, version string, metadata json.RawMessage) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE gfw_probe_nodes
		SET status        = 'online',
		    last_seen_at  = NOW(),
		    agent_version = $2,
		    metadata      = $3,
		    updated_at    = NOW()
		WHERE node_id = $1`,
		nodeID, version, nullJSON(metadata),
	)
	if err != nil {
		return fmt.Errorf("gfw heartbeat %s: %w", nodeID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProbeNodeNotFound
	}
	return nil
}

// MarkOfflineStale sets status=offline for nodes whose last_seen_at is older
// than the given threshold. Used by a background goroutine to detect dead nodes.
func (s *GFWNodeStore) MarkOfflineStale(ctx context.Context, threshold time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE gfw_probe_nodes
		SET status     = 'offline',
		    updated_at = NOW()
		WHERE status     = 'online'
		  AND last_seen_at < NOW() - $1::INTERVAL`,
		fmt.Sprintf("%d seconds", int(threshold.Seconds())),
	)
	if err != nil {
		return 0, fmt.Errorf("gfw mark offline stale: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ── Probe node reads ──────────────────────────────────────────────────────────

// GetByNodeID returns a single probe node by its operator-assigned ID.
func (s *GFWNodeStore) GetByNodeID(ctx context.Context, nodeID string) (*GFWProbeNode, error) {
	var n GFWProbeNode
	err := s.db.GetContext(ctx, &n, `
		SELECT id, uuid, node_id, region, role, status, last_seen_at,
		       agent_version, ip_address, metadata, created_at, updated_at
		FROM gfw_probe_nodes WHERE node_id = $1`, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProbeNodeNotFound
		}
		return nil, fmt.Errorf("get gfw node %s: %w", nodeID, err)
	}
	return &n, nil
}

// List returns all probe nodes, optionally filtered by role.
// role="" returns all roles.
func (s *GFWNodeStore) List(ctx context.Context, role string) ([]GFWProbeNode, error) {
	var out []GFWProbeNode
	err := s.db.SelectContext(ctx, &out, `
		SELECT id, uuid, node_id, region, role, status, last_seen_at,
		       agent_version, ip_address, metadata, created_at, updated_at
		FROM gfw_probe_nodes
		WHERE ($1 = '' OR role = $1)
		ORDER BY region, node_id`, role)
	if err != nil {
		return nil, fmt.Errorf("list gfw nodes: %w", err)
	}
	return out, nil
}

// ── Check assignments ─────────────────────────────────────────────────────────

// UpsertAssignment inserts or replaces the check assignment for a domain.
func (s *GFWNodeStore) UpsertAssignment(ctx context.Context, a *GFWCheckAssignment) error {
	probeIDs, _ := json.Marshal([]string{}) // default empty
	if len(a.ProbeNodeIDs) > 0 {
		probeIDs = a.ProbeNodeIDs
	}
	ctrlIDs, _ := json.Marshal([]string{})
	if len(a.ControlNodeIDs) > 0 {
		ctrlIDs = a.ControlNodeIDs
	}

	return s.db.QueryRowContext(ctx, `
		INSERT INTO gfw_check_assignments
		  (domain_id, probe_node_ids, control_node_ids, check_interval, enabled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (domain_id) DO UPDATE SET
		  probe_node_ids   = EXCLUDED.probe_node_ids,
		  control_node_ids = EXCLUDED.control_node_ids,
		  check_interval   = EXCLUDED.check_interval,
		  enabled          = EXCLUDED.enabled,
		  updated_at       = NOW()
		RETURNING id, uuid, created_at, updated_at`,
		a.DomainID, probeIDs, ctrlIDs, a.CheckInterval, a.Enabled,
	).Scan(&a.ID, &a.UUID, &a.CreatedAt, &a.UpdatedAt)
}

// ListAssignmentsForNode returns all enabled assignments where nodeID appears
// in either probe_node_ids or control_node_ids.
func (s *GFWNodeStore) ListAssignmentsForNode(ctx context.Context, nodeID string) ([]GFWCheckAssignment, error) {
	var out []GFWCheckAssignment
	err := s.db.SelectContext(ctx, &out, `
		SELECT a.id, a.uuid, a.domain_id,
		       a.probe_node_ids, a.control_node_ids,
		       a.check_interval, a.enabled, a.created_at, a.updated_at
		FROM gfw_check_assignments a
		WHERE a.enabled = true
		  AND (
		        a.probe_node_ids   @> $1::JSONB
		     OR a.control_node_ids @> $1::JSONB
		  )
		ORDER BY a.domain_id`,
		fmt.Sprintf(`[%q]`, nodeID),
	)
	if err != nil {
		return nil, fmt.Errorf("list assignments for node %s: %w", nodeID, err)
	}
	return out, nil
}

// ListAllAssignments returns all assignments (admin listing).
func (s *GFWNodeStore) ListAllAssignments(ctx context.Context, enabledOnly bool) ([]GFWCheckAssignment, error) {
	var out []GFWCheckAssignment
	err := s.db.SelectContext(ctx, &out, `
		SELECT id, uuid, domain_id, probe_node_ids, control_node_ids,
		       check_interval, enabled, created_at, updated_at
		FROM gfw_check_assignments
		WHERE ($1 = false OR enabled = true)
		ORDER BY domain_id`, enabledOnly)
	if err != nil {
		return nil, fmt.Errorf("list gfw assignments: %w", err)
	}
	return out, nil
}

// GetAssignmentByDomain returns the assignment for a specific domain.
func (s *GFWNodeStore) GetAssignmentByDomain(ctx context.Context, domainID int64) (*GFWCheckAssignment, error) {
	var a GFWCheckAssignment
	err := s.db.GetContext(ctx, &a, `
		SELECT id, uuid, domain_id, probe_node_ids, control_node_ids,
		       check_interval, enabled, created_at, updated_at
		FROM gfw_check_assignments WHERE domain_id = $1`, domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCheckAssignmentNotFound
		}
		return nil, fmt.Errorf("get assignment for domain %d: %w", domainID, err)
	}
	return &a, nil
}

// DeleteAssignment removes the check assignment for a domain.
func (s *GFWNodeStore) DeleteAssignment(ctx context.Context, domainID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM gfw_check_assignments WHERE domain_id = $1`, domainID)
	if err != nil {
		return fmt.Errorf("delete assignment for domain %d: %w", domainID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCheckAssignmentNotFound
	}
	return nil
}
