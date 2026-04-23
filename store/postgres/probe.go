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

var ErrProbePolicyNotFound = errors.New("probe policy not found")
var ErrProbeTaskNotFound   = errors.New("probe task not found")

// ── Models ────────────────────────────────────────────────────────────────────

// ProbePolicy maps to probe_policies table.
type ProbePolicy struct {
	ID              int64           `db:"id"`
	UUID            string          `db:"uuid"`
	ProjectID       *int64          `db:"project_id"`
	Name            string          `db:"name"`
	Tier            int16           `db:"tier"`
	IntervalSeconds int             `db:"interval_seconds"`
	TimeoutSeconds  int             `db:"timeout_seconds"`
	ExpectedStatus  *int            `db:"expected_status"`
	ExpectedKeyword *string         `db:"expected_keyword"`
	ExpectedMetaTag *string         `db:"expected_meta_tag"`
	TargetFilter    json.RawMessage `db:"target_filter"`
	Enabled         bool            `db:"enabled"`
	CreatedAt       time.Time       `db:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
}

// ProbeTask maps to probe_tasks table.
type ProbeTask struct {
	ID                 int64      `db:"id"`
	UUID               string     `db:"uuid"`
	PolicyID           int64      `db:"policy_id"`
	DomainID           int64      `db:"domain_id"`
	ReleaseID          *int64     `db:"release_id"`
	ExpectedArtifactID *int64     `db:"expected_artifact_id"`
	ScheduledFor       time.Time  `db:"scheduled_for"`
	Status             string     `db:"status"`
	StartedAt          *time.Time `db:"started_at"`
	CompletedAt        *time.Time `db:"completed_at"`
	ErrorMessage       *string    `db:"error_message"`
	CreatedAt          time.Time  `db:"created_at"`
}

// ProbeResult maps to probe_results hypertable (TimescaleDB).
type ProbeResult struct {
	ID                 int64           `db:"id"`
	DomainID           int64           `db:"domain_id"`
	PolicyID           *int64          `db:"policy_id"`
	ProbeTaskID        *int64          `db:"probe_task_id"`
	Tier               int16           `db:"tier"`
	Status             string          `db:"status"`
	HTTPStatus         *int            `db:"http_status"`
	ResponseTimeMS     *int            `db:"response_time_ms"`
	ResponseSizeB      *int            `db:"response_size_b"`
	TLSHandshakeOK     *bool           `db:"tls_handshake_ok"`
	CertExpiresAt      *time.Time      `db:"cert_expires_at"`
	ContentHash        *string         `db:"content_hash"`
	ExpectedArtifactID *int64          `db:"expected_artifact_id"`
	DetectedArtifactID *int64          `db:"detected_artifact_id"`
	ErrorMessage       *string         `db:"error_message"`
	ProbeRunner        *string         `db:"probe_runner"`
	Detail             json.RawMessage `db:"detail"`
	CheckedAt          time.Time       `db:"checked_at"`
}

// ── Store ─────────────────────────────────────────────────────────────────────

type ProbeStore struct {
	db *sqlx.DB
}

func NewProbeStore(db *sqlx.DB) *ProbeStore {
	return &ProbeStore{db: db}
}

// ── Policies ──────────────────────────────────────────────────────────────────

func (s *ProbeStore) ListPolicies(ctx context.Context, projectID *int64, enabledOnly bool) ([]ProbePolicy, error) {
	query := `SELECT id, uuid, project_id, name, tier, interval_seconds, timeout_seconds,
	                 expected_status, expected_keyword, expected_meta_tag, target_filter,
	                 enabled, created_at, updated_at
	          FROM probe_policies
	          WHERE ($1::BIGINT IS NULL OR project_id = $1)
	            AND ($2 = false OR enabled = true)
	          ORDER BY tier, name`
	var out []ProbePolicy
	if err := s.db.SelectContext(ctx, &out, query, projectID, enabledOnly); err != nil {
		return nil, fmt.Errorf("list probe policies: %w", err)
	}
	return out, nil
}

func (s *ProbeStore) GetPolicy(ctx context.Context, id int64) (*ProbePolicy, error) {
	var p ProbePolicy
	err := s.db.GetContext(ctx, &p,
		`SELECT id, uuid, project_id, name, tier, interval_seconds, timeout_seconds,
		        expected_status, expected_keyword, expected_meta_tag, target_filter,
		        enabled, created_at, updated_at
		 FROM probe_policies WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProbePolicyNotFound
		}
		return nil, fmt.Errorf("get probe policy %d: %w", id, err)
	}
	return &p, nil
}

func (s *ProbeStore) CreatePolicy(ctx context.Context, p *ProbePolicy) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO probe_policies
		   (project_id, name, tier, interval_seconds, timeout_seconds,
		    expected_status, expected_keyword, expected_meta_tag, target_filter, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, uuid, created_at, updated_at`,
		p.ProjectID, p.Name, p.Tier, p.IntervalSeconds, p.TimeoutSeconds,
		p.ExpectedStatus, p.ExpectedKeyword, p.ExpectedMetaTag, nullJSON(p.TargetFilter), p.Enabled,
	).Scan(&p.ID, &p.UUID, &p.CreatedAt, &p.UpdatedAt)
}

func (s *ProbeStore) UpdatePolicy(ctx context.Context, p *ProbePolicy) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE probe_policies SET
		   name=$1, tier=$2, interval_seconds=$3, timeout_seconds=$4,
		   expected_status=$5, expected_keyword=$6, expected_meta_tag=$7,
		   target_filter=$8, enabled=$9, updated_at=NOW()
		 WHERE id=$10`,
		p.Name, p.Tier, p.IntervalSeconds, p.TimeoutSeconds,
		p.ExpectedStatus, p.ExpectedKeyword, p.ExpectedMetaTag,
		nullJSON(p.TargetFilter), p.Enabled, p.ID)
	if err != nil {
		return fmt.Errorf("update probe policy %d: %w", p.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProbePolicyNotFound
	}
	return nil
}

func (s *ProbeStore) DeletePolicy(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM probe_policies WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete probe policy %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProbePolicyNotFound
	}
	return nil
}

// ── Tasks ─────────────────────────────────────────────────────────────────────

// CreateTask inserts a new probe_task row.
func (s *ProbeStore) CreateTask(ctx context.Context, t *ProbeTask) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO probe_tasks
		   (policy_id, domain_id, release_id, expected_artifact_id, scheduled_for, status)
		 VALUES ($1,$2,$3,$4,$5,'pending')
		 RETURNING id, uuid, created_at`,
		t.PolicyID, t.DomainID, t.ReleaseID, t.ExpectedArtifactID, t.ScheduledFor,
	).Scan(&t.ID, &t.UUID, &t.CreatedAt)
}

// ListPendingTasks returns probe_tasks that are due (scheduled_for <= now) and still pending.
func (s *ProbeStore) ListPendingTasks(ctx context.Context, limit int) ([]ProbeTask, error) {
	var out []ProbeTask
	err := s.db.SelectContext(ctx, &out,
		`SELECT id, uuid, policy_id, domain_id, release_id, expected_artifact_id,
		        scheduled_for, status, started_at, completed_at, error_message, created_at
		 FROM probe_tasks
		 WHERE status = 'pending' AND scheduled_for <= NOW()
		 ORDER BY scheduled_for ASC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending probe tasks: %w", err)
	}
	return out, nil
}

// ClaimTask transitions a task from pending → running atomically.
// Returns ErrProbeTaskNotFound if already claimed by another worker.
func (s *ProbeStore) ClaimTask(ctx context.Context, taskID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE probe_tasks SET status='running', started_at=NOW()
		 WHERE id=$1 AND status='pending'`, taskID)
	if err != nil {
		return fmt.Errorf("claim probe task %d: %w", taskID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrProbeTaskNotFound // already claimed or doesn't exist
	}
	return nil
}

// CompleteTask transitions running → completed or cancelled, recording outcome.
func (s *ProbeStore) CompleteTask(ctx context.Context, taskID int64, errMsg *string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE probe_tasks SET status='completed', completed_at=NOW(), error_message=$1
		 WHERE id=$2`, errMsg, taskID)
	if err != nil {
		return fmt.Errorf("complete probe task %d: %w", taskID, err)
	}
	return nil
}

// ── Results ───────────────────────────────────────────────────────────────────

// SaveResult inserts a probe_result row into the TimescaleDB hypertable.
func (s *ProbeStore) SaveResult(ctx context.Context, r *ProbeResult) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO probe_results
		   (domain_id, policy_id, probe_task_id, tier, status,
		    http_status, response_time_ms, response_size_b,
		    tls_handshake_ok, cert_expires_at, content_hash,
		    expected_artifact_id, detected_artifact_id,
		    error_message, probe_runner, detail, checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		 RETURNING id`,
		r.DomainID, r.PolicyID, r.ProbeTaskID, r.Tier, r.Status,
		r.HTTPStatus, r.ResponseTimeMS, r.ResponseSizeB,
		r.TLSHandshakeOK, r.CertExpiresAt, r.ContentHash,
		r.ExpectedArtifactID, r.DetectedArtifactID,
		r.ErrorMessage, r.ProbeRunner, nullJSON(r.Detail), r.CheckedAt,
	).Scan(&r.ID)
}

// ListDomainResults returns the most recent probe results for a domain.
func (s *ProbeStore) ListDomainResults(ctx context.Context, domainID int64, tier *int16, limit int) ([]ProbeResult, error) {
	var out []ProbeResult
	err := s.db.SelectContext(ctx, &out,
		`SELECT id, domain_id, policy_id, probe_task_id, tier, status,
		        http_status, response_time_ms, response_size_b,
		        tls_handshake_ok, cert_expires_at, content_hash,
		        expected_artifact_id, detected_artifact_id,
		        error_message, probe_runner, detail, checked_at
		 FROM probe_results
		 WHERE domain_id = $1
		   AND ($2::SMALLINT IS NULL OR tier = $2)
		 ORDER BY checked_at DESC
		 LIMIT $3`, domainID, tier, limit)
	if err != nil {
		return nil, fmt.Errorf("list probe results domain %d: %w", domainID, err)
	}
	return out, nil
}

// DomainLastResult returns the single most recent probe_result for a domain+tier.
func (s *ProbeStore) DomainLastResult(ctx context.Context, domainID int64, tier int16) (*ProbeResult, error) {
	var r ProbeResult
	err := s.db.GetContext(ctx, &r,
		`SELECT id, domain_id, policy_id, probe_task_id, tier, status,
		        http_status, response_time_ms, response_size_b,
		        tls_handshake_ok, cert_expires_at, content_hash,
		        expected_artifact_id, detected_artifact_id,
		        error_message, probe_runner, detail, checked_at
		 FROM probe_results
		 WHERE domain_id=$1 AND tier=$2
		 ORDER BY checked_at DESC
		 LIMIT 1`, domainID, tier)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no result yet is not an error
		}
		return nil, fmt.Errorf("domain last probe result: %w", err)
	}
	return &r, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// nullJSON returns nil if b is empty/null, otherwise returns b unchanged.
// Prevents inserting the literal string "null" as JSONB.
func nullJSON(b json.RawMessage) interface{} {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return b
}
