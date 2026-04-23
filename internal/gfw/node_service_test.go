package gfw

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// ── Test doubles ──────────────────────────────────────────────────────────────

type stubNodeStore struct {
	registerFn         func(ctx context.Context, node *postgres.GFWProbeNode) error
	heartbeatFn        func(ctx context.Context, nodeID, version string, meta json.RawMessage) error
	getByNodeIDFn      func(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error)
	listFn             func(ctx context.Context, role string) ([]postgres.GFWProbeNode, error)
	listAssignmentsFn  func(ctx context.Context, nodeID string) ([]postgres.GFWCheckAssignment, error)
	markOfflineStaleFn func(ctx context.Context, threshold time.Duration) (int64, error)
}

func (s *stubNodeStore) Register(ctx context.Context, node *postgres.GFWProbeNode) error {
	return s.registerFn(ctx, node)
}
func (s *stubNodeStore) Heartbeat(ctx context.Context, nodeID, version string, meta json.RawMessage) error {
	return s.heartbeatFn(ctx, nodeID, version, meta)
}
func (s *stubNodeStore) GetByNodeID(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error) {
	return s.getByNodeIDFn(ctx, nodeID)
}
func (s *stubNodeStore) List(ctx context.Context, role string) ([]postgres.GFWProbeNode, error) {
	return s.listFn(ctx, role)
}
func (s *stubNodeStore) ListAssignmentsForNode(ctx context.Context, nodeID string) ([]postgres.GFWCheckAssignment, error) {
	return s.listAssignmentsFn(ctx, nodeID)
}
func (s *stubNodeStore) MarkOfflineStale(ctx context.Context, threshold time.Duration) (int64, error) {
	return s.markOfflineStaleFn(ctx, threshold)
}

type stubDomainGetter struct {
	getByIDFn func(ctx context.Context, id int64) (*postgres.Domain, error)
}

func (s *stubDomainGetter) GetByID(ctx context.Context, id int64) (*postgres.Domain, error) {
	return s.getByIDFn(ctx, id)
}

// newTestService builds a NodeService backed by stubs.
func newTestService(ns nodeStore, ds domainGetter) *NodeService {
	return &NodeService{store: ns, domain: ds, logger: zap.NewNop()}
}

// ── Register tests ────────────────────────────────────────────────────────────

func TestNodeService_Register_Valid(t *testing.T) {
	var capturedNode *postgres.GFWProbeNode
	svc := newTestService(&stubNodeStore{
		registerFn: func(_ context.Context, node *postgres.GFWProbeNode) error {
			capturedNode = node
			return nil
		},
	}, nil)

	resp, err := svc.Register(context.Background(), probeprotocol.RegisterRequest{
		NodeID:       "cn-beijing-01",
		Region:       "cn-north",
		Role:         probeprotocol.RoleProbe,
		ProbeVersion: "0.1.0",
	})

	require.NoError(t, err)
	require.NotNil(t, capturedNode)
	assert.Equal(t, "cn-beijing-01", capturedNode.NodeID)
	assert.Equal(t, probeprotocol.RoleProbe, capturedNode.Role)
	assert.Equal(t, "cn-beijing-01", resp.NodeID)
	assert.Equal(t, probeprotocol.StatusOnline, resp.Status)
	assert.Equal(t, 30, resp.HeartbeatSecs)
	assert.Equal(t, 180, resp.CheckInterval)
}

func TestNodeService_Register_MissingNodeID(t *testing.T) {
	svc := newTestService(nil, nil)
	_, err := svc.Register(context.Background(), probeprotocol.RegisterRequest{
		Region: "cn-north",
		Role:   probeprotocol.RoleProbe,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "node_id is required")
}

func TestNodeService_Register_InvalidRole(t *testing.T) {
	svc := newTestService(nil, nil)
	_, err := svc.Register(context.Background(), probeprotocol.RegisterRequest{
		NodeID: "node-01",
		Role:   "agent", // invalid
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")
}

func TestNodeService_Register_BothRoles(t *testing.T) {
	for _, role := range []string{probeprotocol.RoleProbe, probeprotocol.RoleControl} {
		t.Run(role, func(t *testing.T) {
			svc := newTestService(&stubNodeStore{
				registerFn: func(_ context.Context, _ *postgres.GFWProbeNode) error { return nil },
			}, nil)
			resp, err := svc.Register(context.Background(), probeprotocol.RegisterRequest{
				NodeID: "node-01", Region: "hk", Role: role,
			})
			require.NoError(t, err)
			assert.Equal(t, probeprotocol.StatusOnline, resp.Status)
		})
	}
}

func TestNodeService_Register_StoreError(t *testing.T) {
	svc := newTestService(&stubNodeStore{
		registerFn: func(_ context.Context, _ *postgres.GFWProbeNode) error {
			return errors.New("db connection failed")
		},
	}, nil)

	_, err := svc.Register(context.Background(), probeprotocol.RegisterRequest{
		NodeID: "node-01",
		Role:   probeprotocol.RoleProbe,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "register node")
}

// ── Heartbeat tests ───────────────────────────────────────────────────────────

func TestNodeService_Heartbeat_OK(t *testing.T) {
	var capturedNodeID string
	svc := newTestService(&stubNodeStore{
		heartbeatFn: func(_ context.Context, nodeID, _ string, _ json.RawMessage) error {
			capturedNodeID = nodeID
			return nil
		},
	}, nil)

	resp, err := svc.Heartbeat(context.Background(), probeprotocol.HeartbeatRequest{
		NodeID:       "cn-beijing-01",
		ProbeVersion: "0.1.0",
		LoadAvg1:     0.5,
		ChecksToday:  42,
	})
	require.NoError(t, err)
	assert.Equal(t, "cn-beijing-01", capturedNodeID)
	assert.True(t, resp.Ack)
}

func TestNodeService_Heartbeat_NodeNotFound(t *testing.T) {
	svc := newTestService(&stubNodeStore{
		heartbeatFn: func(_ context.Context, _ string, _ string, _ json.RawMessage) error {
			return postgres.ErrProbeNodeNotFound
		},
	}, nil)

	_, err := svc.Heartbeat(context.Background(), probeprotocol.HeartbeatRequest{NodeID: "ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestNodeService_Heartbeat_MetadataEncoded(t *testing.T) {
	var capturedMeta json.RawMessage
	svc := newTestService(&stubNodeStore{
		heartbeatFn: func(_ context.Context, _ string, _ string, meta json.RawMessage) error {
			capturedMeta = meta
			return nil
		},
	}, nil)

	_, err := svc.Heartbeat(context.Background(), probeprotocol.HeartbeatRequest{
		NodeID:      "node-01",
		LoadAvg1:    1.23,
		DiskFreePct: 80.5,
		ChecksToday: 5,
	})
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedMeta, &m))
	assert.Equal(t, 1.23, m["load_avg_1"])
	assert.Equal(t, float64(5), m["checks_today"])
}

// ── GetAssignments tests ──────────────────────────────────────────────────────

func TestNodeService_GetAssignments_OK(t *testing.T) {
	svc := newTestService(
		&stubNodeStore{
			getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
				return &postgres.GFWProbeNode{NodeID: "node-01"}, nil
			},
			listAssignmentsFn: func(_ context.Context, _ string) ([]postgres.GFWCheckAssignment, error) {
				return []postgres.GFWCheckAssignment{
					{ID: 10, DomainID: 100, CheckInterval: 180},
					{ID: 11, DomainID: 200, CheckInterval: 300},
				}, nil
			},
		},
		&stubDomainGetter{
			getByIDFn: func(_ context.Context, id int64) (*postgres.Domain, error) {
				fqdns := map[int64]string{100: "example.com", 200: "other.com"}
				return &postgres.Domain{ID: id, FQDN: fqdns[id]}, nil
			},
		},
	)

	resp, err := svc.GetAssignments(context.Background(), "node-01")
	require.NoError(t, err)
	assert.Equal(t, "node-01", resp.NodeID)
	require.Len(t, resp.Assignments, 2)
	assert.Equal(t, int64(10), resp.Assignments[0].AssignmentID)
	assert.Equal(t, "example.com", resp.Assignments[0].FQDN)
	assert.Equal(t, 180, resp.Assignments[0].CheckInterval)
}

func TestNodeService_GetAssignments_NodeNotFound(t *testing.T) {
	svc := newTestService(&stubNodeStore{
		getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
			return nil, postgres.ErrProbeNodeNotFound
		},
	}, nil)

	_, err := svc.GetAssignments(context.Background(), "ghost")
	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrProbeNodeNotFound) ||
		contains(err.Error(), "probe node not found"))
}

func TestNodeService_GetAssignments_SkipsMissingDomain(t *testing.T) {
	svc := newTestService(
		&stubNodeStore{
			getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
				return &postgres.GFWProbeNode{NodeID: "node-01"}, nil
			},
			listAssignmentsFn: func(_ context.Context, _ string) ([]postgres.GFWCheckAssignment, error) {
				return []postgres.GFWCheckAssignment{
					{ID: 1, DomainID: 999}, // will 404
					{ID: 2, DomainID: 100},
				}, nil
			},
		},
		&stubDomainGetter{
			getByIDFn: func(_ context.Context, id int64) (*postgres.Domain, error) {
				if id == 999 {
					return nil, errors.New("domain not found")
				}
				return &postgres.Domain{ID: id, FQDN: "ok.example.com"}, nil
			},
		},
	)

	resp, err := svc.GetAssignments(context.Background(), "node-01")
	require.NoError(t, err)
	// domain 999 skipped; only domain 100 included
	require.Len(t, resp.Assignments, 1)
	assert.Equal(t, int64(100), resp.Assignments[0].DomainID)
}

func TestNodeService_GetAssignments_EmptyAssignments(t *testing.T) {
	svc := newTestService(
		&stubNodeStore{
			getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
				return &postgres.GFWProbeNode{NodeID: "node-01"}, nil
			},
			listAssignmentsFn: func(_ context.Context, _ string) ([]postgres.GFWCheckAssignment, error) {
				return nil, nil
			},
		},
		nil,
	)

	resp, err := svc.GetAssignments(context.Background(), "node-01")
	require.NoError(t, err)
	assert.Empty(t, resp.Assignments)
}

// ── MarkStaleOffline tests ────────────────────────────────────────────────────

func TestNodeService_MarkStaleOffline_PassesCorrectThreshold(t *testing.T) {
	var capturedThreshold time.Duration
	svc := newTestService(&stubNodeStore{
		markOfflineStaleFn: func(_ context.Context, threshold time.Duration) (int64, error) {
			capturedThreshold = threshold
			return 0, nil
		},
	}, nil)

	require.NoError(t, svc.MarkStaleOffline(context.Background()))
	assert.Equal(t, offlineThreshold, capturedThreshold)
}

func TestNodeService_MarkStaleOffline_ReturnsErrorOnStoreFailure(t *testing.T) {
	svc := newTestService(&stubNodeStore{
		markOfflineStaleFn: func(_ context.Context, _ time.Duration) (int64, error) {
			return 0, errors.New("db error")
		},
	}, nil)

	err := svc.MarkStaleOffline(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mark stale offline")
}

func TestNodeService_MarkStaleOffline_LogsWhenNodesGoOffline(t *testing.T) {
	svc := newTestService(&stubNodeStore{
		markOfflineStaleFn: func(_ context.Context, _ time.Duration) (int64, error) {
			return 5, nil // 5 nodes went offline
		},
	}, nil)

	// Should not return an error even when nodes go offline (just warns)
	require.NoError(t, svc.MarkStaleOffline(context.Background()))
}

// ── offlineThreshold constant test ───────────────────────────────────────────

func TestOfflineThreshold(t *testing.T) {
	assert.Equal(t, 90*time.Second, offlineThreshold,
		"offlineThreshold must be 90s per acceptance criterion")
}

// contains is a helper to check substring without importing strings.
func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
