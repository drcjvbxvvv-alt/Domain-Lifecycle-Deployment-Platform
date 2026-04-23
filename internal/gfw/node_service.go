// Package gfw implements the GFW (Great Firewall) detection vertical.
// Phase D: probe node management, measurement collection, blocking analysis.
package gfw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// offlineThreshold is how long a node can go without a heartbeat before it
// is considered offline. Matches the acceptance criterion: offline > 90s.
const offlineThreshold = 90 * time.Second

// nodeStore is the subset of *postgres.GFWNodeStore used by NodeService.
// Defined internally to allow unit testing without a live database.
type nodeStore interface {
	Register(ctx context.Context, node *postgres.GFWProbeNode) error
	Heartbeat(ctx context.Context, nodeID, version string, metadata json.RawMessage) error
	GetByNodeID(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error)
	List(ctx context.Context, role string) ([]postgres.GFWProbeNode, error)
	ListAssignmentsForNode(ctx context.Context, nodeID string) ([]postgres.GFWCheckAssignment, error)
	MarkOfflineStale(ctx context.Context, threshold time.Duration) (int64, error)
}

// domainGetter is the subset of *postgres.DomainStore used by NodeService.
type domainGetter interface {
	GetByID(ctx context.Context, id int64) (*postgres.Domain, error)
}

// NodeService is the single write path for gfw_probe_nodes.
// All status mutations go through this service.
type NodeService struct {
	store  nodeStore
	domain domainGetter
	logger *zap.Logger
}

func NewNodeService(
	store *postgres.GFWNodeStore,
	domain *postgres.DomainStore,
	logger *zap.Logger,
) *NodeService {
	return &NodeService{store: store, domain: domain, logger: logger}
}

// Register upserts the probe node and returns the protocol response.
// Idempotent — probe binaries call this on every restart.
func (s *NodeService) Register(ctx context.Context, req probeprotocol.RegisterRequest) (*probeprotocol.RegisterResponse, error) {
	if req.NodeID == "" {
		return nil, fmt.Errorf("register: node_id is required")
	}
	if req.Role != probeprotocol.RoleProbe && req.Role != probeprotocol.RoleControl {
		return nil, fmt.Errorf("register: invalid role %q (must be probe or control)", req.Role)
	}

	node := &postgres.GFWProbeNode{
		NodeID: req.NodeID,
		Region: req.Region,
		Role:   req.Role,
	}
	if req.ProbeVersion != "" {
		node.AgentVersion = &req.ProbeVersion
	}
	if req.IPAddress != "" {
		node.IPAddress = &req.IPAddress
	}

	if err := s.store.Register(ctx, node); err != nil {
		return nil, fmt.Errorf("register node %s: %w", req.NodeID, err)
	}

	s.logger.Info("probe node registered",
		zap.String("node_id", node.NodeID),
		zap.String("region", node.Region),
		zap.String("role", node.Role),
		zap.String("version", req.ProbeVersion),
	)

	return &probeprotocol.RegisterResponse{
		NodeID:        node.NodeID,
		Status:        probeprotocol.StatusOnline,
		HeartbeatSecs: 30,
		CheckInterval: 180,
	}, nil
}

// Heartbeat updates last_seen_at and optional metadata for the node.
func (s *NodeService) Heartbeat(ctx context.Context, req probeprotocol.HeartbeatRequest) (*probeprotocol.HeartbeatResponse, error) {
	meta, _ := json.Marshal(map[string]interface{}{
		"load_avg_1":    req.LoadAvg1,
		"disk_free_pct": req.DiskFreePct,
		"checks_today":  req.ChecksToday,
	})

	if err := s.store.Heartbeat(ctx, req.NodeID, req.ProbeVersion, meta); err != nil {
		if errors.Is(err, postgres.ErrProbeNodeNotFound) {
			return nil, fmt.Errorf("heartbeat: node %q not registered", req.NodeID)
		}
		return nil, fmt.Errorf("heartbeat node %s: %w", req.NodeID, err)
	}

	return &probeprotocol.HeartbeatResponse{
		Ack:           true,
		HasNewDomains: false, // TODO: track assignment version for incremental updates
	}, nil
}

// GetAssignments returns the list of domains assigned to a probe node.
// The response includes all enabled assignments where the node appears in
// probe_node_ids or control_node_ids.
func (s *NodeService) GetAssignments(ctx context.Context, nodeID string) (*probeprotocol.AssignmentsResponse, error) {
	// Verify node exists
	node, err := s.store.GetByNodeID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get assignments: %w", err)
	}

	rows, err := s.store.ListAssignmentsForNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list assignments for %s: %w", nodeID, err)
	}

	// Resolve FQDNs for assigned domain IDs.
	// We call DomainStore.GetByID for each; for PD.1 scale (≤200 domains) this
	// is acceptable. PD.2+ can batch this.
	assignments := make([]probeprotocol.Assignment, 0, len(rows))
	for _, row := range rows {
		domain, err := s.domain.GetByID(ctx, row.DomainID)
		if err != nil {
			s.logger.Warn("gfw get assignments: domain not found",
				zap.Int64("domain_id", row.DomainID),
				zap.String("node_id", node.NodeID),
				zap.Error(err),
			)
			continue
		}
		assignments = append(assignments, probeprotocol.Assignment{
			AssignmentID:  row.ID,
			DomainID:      row.DomainID,
			FQDN:          domain.FQDN,
			CheckInterval: row.CheckInterval,
		})
	}

	return &probeprotocol.AssignmentsResponse{
		NodeID:      nodeID,
		Assignments: assignments,
	}, nil
}

// MarkStaleOffline transitions nodes that have not heartbeated within
// offlineThreshold to "offline". Called periodically by the server.
func (s *NodeService) MarkStaleOffline(ctx context.Context) error {
	n, err := s.store.MarkOfflineStale(ctx, offlineThreshold)
	if err != nil {
		return fmt.Errorf("mark stale offline: %w", err)
	}
	if n > 0 {
		s.logger.Warn("gfw probe nodes went offline",
			zap.Int64("count", n),
			zap.Duration("threshold", offlineThreshold),
		)
	}
	return nil
}

// ListNodes returns all probe nodes, optionally filtered by role.
func (s *NodeService) ListNodes(ctx context.Context, role string) ([]postgres.GFWProbeNode, error) {
	return s.store.List(ctx, role)
}

// GetNode returns a single probe node.
func (s *NodeService) GetNode(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error) {
	return s.store.GetByNodeID(ctx, nodeID)
}
