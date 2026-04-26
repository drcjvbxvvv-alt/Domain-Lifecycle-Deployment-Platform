package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"domain-platform/pkg/probeprotocol"
)

// ErrGFWMeasurementNotFound is returned when a measurement row cannot be found.
var ErrGFWMeasurementNotFound = errors.New("gfw measurement not found")

// GFWMeasurement is a single 4-layer probe record stored in gfw_measurements.
// All per-layer results are stored as JSONB and decoded on retrieval.
type GFWMeasurement struct {
	ID          int64           `db:"id"`
	DomainID    int64           `db:"domain_id"`
	NodeID      string          `db:"node_id"`
	NodeRole    string          `db:"node_role"`
	Region      string          `db:"region"`
	FQDN        string          `db:"fqdn"`
	DNSResult   json.RawMessage `db:"dns_result"`
	TCPResults  json.RawMessage `db:"tcp_results"`
	TLSResults  json.RawMessage `db:"tls_results"`
	HTTPResult  json.RawMessage `db:"http_result"`
	TotalMS     *int            `db:"total_ms"`
	MeasuredAt  time.Time       `db:"measured_at"`
}

// GFWMeasurementStore provides read/write access to the gfw_measurements table.
type GFWMeasurementStore struct {
	db *sqlx.DB
}

// NewGFWMeasurementStore creates a GFWMeasurementStore backed by db.
func NewGFWMeasurementStore(db *sqlx.DB) *GFWMeasurementStore {
	return &GFWMeasurementStore{db: db}
}

// InsertMeasurements bulk-inserts a slice of Measurement values received from
// a probe node.  The insert is batched in a single transaction.
// nodeRegion is looked up by the caller (from gfw_probe_nodes) and passed in
// so we avoid per-row subqueries inside the transaction.
func (s *GFWMeasurementStore) InsertMeasurements(ctx context.Context, measurements []probeprotocol.Measurement, nodeRegion string) error {
	if len(measurements) == 0 {
		return nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	const query = `
		INSERT INTO gfw_measurements
			(domain_id, node_id, node_role, region, fqdn,
			 dns_result, tcp_results, tls_results, http_result,
			 total_ms, measured_at)
		VALUES
			($1, $2, $3, $4, $5,
			 $6, $7, $8, $9,
			 $10, $11)`

	for i := range measurements {
		m := &measurements[i]

		dnsJSON, err := marshalOrNull(m.DNS)
		if err != nil {
			return fmt.Errorf("marshal dns result: %w", err)
		}
		tcpJSON, err := marshalOrNull(m.TCP)
		if err != nil {
			return fmt.Errorf("marshal tcp results: %w", err)
		}
		tlsJSON, err := marshalOrNull(m.TLS)
		if err != nil {
			return fmt.Errorf("marshal tls results: %w", err)
		}
		httpJSON, err := marshalOrNull(m.HTTP)
		if err != nil {
			return fmt.Errorf("marshal http result: %w", err)
		}

		measuredAt := m.MeasuredAt
		if measuredAt.IsZero() {
			measuredAt = time.Now().UTC()
		}

		totalMS := (*int)(nil)
		if m.TotalMS > 0 {
			v := int(m.TotalMS)
			totalMS = &v
		}

		if _, err := tx.ExecContext(ctx, query,
			m.DomainID, m.NodeID, m.NodeRole, nodeRegion, m.FQDN,
			dnsJSON, tcpJSON, tlsJSON, httpJSON,
			totalMS, measuredAt,
		); err != nil {
			return fmt.Errorf("insert measurement (fqdn=%s): %w", m.FQDN, err)
		}
	}

	return tx.Commit()
}

// ListMeasurements returns measurements for domainID in descending time order.
// from/to are inclusive; pass zero values to skip the bound.
// limit ≤ 0 defaults to 100.
func (s *GFWMeasurementStore) ListMeasurements(
	ctx context.Context,
	domainID int64,
	from, to time.Time,
	limit int,
) ([]GFWMeasurement, error) {
	if limit <= 0 {
		limit = 100
	}

	args := []any{domainID, limit}
	conds := "domain_id = $1"
	argIdx := 3

	if !from.IsZero() {
		conds += fmt.Sprintf(" AND measured_at >= $%d", argIdx)
		args = append(args, from)
		argIdx++
	}
	if !to.IsZero() {
		conds += fmt.Sprintf(" AND measured_at <= $%d", argIdx)
		args = append(args, to)
	}

	query := fmt.Sprintf(`
		SELECT id, domain_id, node_id, node_role, region, fqdn,
		       dns_result, tcp_results, tls_results, http_result,
		       total_ms, measured_at
		FROM gfw_measurements
		WHERE %s
		ORDER BY measured_at DESC
		LIMIT $2`, conds)

	var rows []GFWMeasurement
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list gfw measurements: %w", err)
	}
	return rows, nil
}

// LatestMeasurementPair returns the most recent measurement from a probe node
// and the most recent measurement from a control node for the given domain.
// Either value may be nil if no measurement exists for that role.
func (s *GFWMeasurementStore) LatestMeasurementPair(
	ctx context.Context,
	domainID int64,
) (probe *GFWMeasurement, control *GFWMeasurement, err error) {
	const query = `
		SELECT DISTINCT ON (node_role)
			id, domain_id, node_id, node_role, region, fqdn,
			dns_result, tcp_results, tls_results, http_result,
			total_ms, measured_at
		FROM gfw_measurements
		WHERE domain_id = $1
		  AND node_role IN ('probe', 'control')
		ORDER BY node_role, measured_at DESC`

	var rows []GFWMeasurement
	if err := s.db.SelectContext(ctx, &rows, query, domainID); err != nil {
		return nil, nil, fmt.Errorf("latest measurement pair: %w", err)
	}

	for i := range rows {
		switch rows[i].NodeRole {
		case probeprotocol.RoleProbe:
			probe = &rows[i]
		case probeprotocol.RoleControl:
			control = &rows[i]
		}
	}
	return probe, control, nil
}

// LatestMeasurementsByNode returns the most recent measurement per node for
// domainID, ordered by measured_at DESC.  Used by PD.3 confidence scoring.
func (s *GFWMeasurementStore) LatestMeasurementsByNode(
	ctx context.Context,
	domainID int64,
	limit int,
) ([]GFWMeasurement, error) {
	if limit <= 0 {
		limit = 10
	}
	const query = `
		SELECT DISTINCT ON (node_id)
			id, domain_id, node_id, node_role, region, fqdn,
			dns_result, tcp_results, tls_results, http_result,
			total_ms, measured_at
		FROM gfw_measurements
		WHERE domain_id = $1
		ORDER BY node_id, measured_at DESC
		LIMIT $2`

	var rows []GFWMeasurement
	if err := s.db.SelectContext(ctx, &rows, query, domainID, limit); err != nil {
		return nil, fmt.Errorf("latest measurements by node: %w", err)
	}
	return rows, nil
}

// ListBogonIPs returns all IPs from the gfw_bogon_ips table.
func (s *GFWMeasurementStore) ListBogonIPs(ctx context.Context) ([]string, error) {
	var ips []string
	const query = `SELECT ip_address FROM gfw_bogon_ips ORDER BY id`
	if err := s.db.SelectContext(ctx, &ips, query); err != nil {
		return nil, fmt.Errorf("list bogon ips: %w", err)
	}
	return ips, nil
}

// InsertBogonIP adds a new operator-defined bogon IP.
// Duplicate IPs are silently ignored.
func (s *GFWMeasurementStore) InsertBogonIP(ctx context.Context, ip, note string) error {
	const query = `
		INSERT INTO gfw_bogon_ips (ip_address, source, note)
		VALUES ($1, 'operator', $2)
		ON CONFLICT (ip_address) DO NOTHING`
	if _, err := s.db.ExecContext(ctx, query, ip, note); err != nil {
		return fmt.Errorf("insert bogon ip: %w", err)
	}
	return nil
}

// DeleteBogonIP removes an operator-defined bogon IP.
// Seeded IPs cannot be removed via this method (source='seeded').
func (s *GFWMeasurementStore) DeleteBogonIP(ctx context.Context, ip string) error {
	const query = `DELETE FROM gfw_bogon_ips WHERE ip_address = $1 AND source = 'operator'`
	res, err := s.db.ExecContext(ctx, query, ip)
	if err != nil {
		return fmt.Errorf("delete bogon ip: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("bogon ip %q not found or is seeded (cannot delete)", ip)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// marshalOrNull marshals v to JSON; returns nil (SQL NULL) when v is nil.
func marshalOrNull(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Treat empty slices as NULL to keep JSONB compact.
	if string(b) == "null" || string(b) == "[]" {
		return nil, nil
	}
	return b, nil
}
