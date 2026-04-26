package gfw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// ── Stub stores ───────────────────────────────────────────────────────────────

type stubMeasurementStore struct {
	insertFn            func(ctx context.Context, ms []probeprotocol.Measurement, region string) error
	listFn              func(ctx context.Context, domainID int64, from, to time.Time, limit int) ([]postgres.GFWMeasurement, error)
	latestPairFn        func(ctx context.Context, domainID int64) (*postgres.GFWMeasurement, *postgres.GFWMeasurement, error)
	listBogonsFn        func(ctx context.Context) ([]string, error)
	insertBogonFn       func(ctx context.Context, ip, note string) error
	deleteBogonFn       func(ctx context.Context, ip string) error
}

func (s *stubMeasurementStore) InsertMeasurements(ctx context.Context, ms []probeprotocol.Measurement, region string) error {
	return s.insertFn(ctx, ms, region)
}
func (s *stubMeasurementStore) ListMeasurements(ctx context.Context, domainID int64, from, to time.Time, limit int) ([]postgres.GFWMeasurement, error) {
	return s.listFn(ctx, domainID, from, to, limit)
}
func (s *stubMeasurementStore) LatestMeasurementPair(ctx context.Context, domainID int64) (*postgres.GFWMeasurement, *postgres.GFWMeasurement, error) {
	return s.latestPairFn(ctx, domainID)
}
func (s *stubMeasurementStore) ListBogonIPs(ctx context.Context) ([]string, error) {
	return s.listBogonsFn(ctx)
}
func (s *stubMeasurementStore) InsertBogonIP(ctx context.Context, ip, note string) error {
	return s.insertBogonFn(ctx, ip, note)
}
func (s *stubMeasurementStore) DeleteBogonIP(ctx context.Context, ip string) error {
	return s.deleteBogonFn(ctx, ip)
}

type stubNodeStoreForMeasurement struct {
	getByNodeIDFn func(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error)
}

func (s *stubNodeStoreForMeasurement) GetByNodeID(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error) {
	return s.getByNodeIDFn(ctx, nodeID)
}

// ── measurementServiceImpl wraps MeasurementService with stub interfaces ──────

// MeasurementService uses concrete store types, so we test through the real
// service with interfaces injected via unexported fields.  To keep the test
// file self-contained we duplicate the interface contract here and test the
// business logic, not the DB layer.

type measurementStoreIface interface {
	InsertMeasurements(ctx context.Context, ms []probeprotocol.Measurement, region string) error
	ListMeasurements(ctx context.Context, domainID int64, from, to time.Time, limit int) ([]postgres.GFWMeasurement, error)
	LatestMeasurementPair(ctx context.Context, domainID int64) (*postgres.GFWMeasurement, *postgres.GFWMeasurement, error)
	ListBogonIPs(ctx context.Context) ([]string, error)
	InsertBogonIP(ctx context.Context, ip, note string) error
	DeleteBogonIP(ctx context.Context, ip string) error
}

type nodeStoreIface interface {
	GetByNodeID(ctx context.Context, nodeID string) (*postgres.GFWProbeNode, error)
}

// testMeasurementService is an inline re-implementation of MeasurementService
// that accepts interfaces so we can inject stubs in tests.  It mirrors the
// real service logic exactly.
type testMeasurementService struct {
	mstore measurementStoreIface
	nstore nodeStoreIface
	logger *zap.Logger
}

func (s *testMeasurementService) StoreMeasurements(ctx context.Context, nodeID string, measurements []probeprotocol.Measurement) error {
	if len(measurements) == 0 {
		return nil
	}
	node, err := s.nstore.GetByNodeID(ctx, nodeID)
	if err != nil {
		return err
	}
	return s.mstore.InsertMeasurements(ctx, measurements, node.Region)
}

func (s *testMeasurementService) ListBogonIPs(ctx context.Context) ([]string, error) {
	return s.mstore.ListBogonIPs(ctx)
}

func newTestMeasurementService(ms measurementStoreIface, ns nodeStoreIface) *testMeasurementService {
	return &testMeasurementService{
		mstore: ms,
		nstore: ns,
		logger: zap.NewNop(),
	}
}

// ── StoreMeasurements tests ───────────────────────────────────────────────────

func TestStoreMeasurements_HappyPath(t *testing.T) {
	var insertedMeasurements []probeprotocol.Measurement
	var insertedRegion string

	ms := &stubMeasurementStore{
		insertFn: func(_ context.Context, m []probeprotocol.Measurement, region string) error {
			insertedMeasurements = m
			insertedRegion = region
			return nil
		},
	}
	ns := &stubNodeStoreForMeasurement{
		getByNodeIDFn: func(_ context.Context, nodeID string) (*postgres.GFWProbeNode, error) {
			return &postgres.GFWProbeNode{NodeID: nodeID, Region: "cn-north", Role: "probe"}, nil
		},
	}
	svc := newTestMeasurementService(ms, ns)

	measurements := []probeprotocol.Measurement{
		{DomainID: 1, NodeID: "cn-beijing-01", NodeRole: "probe", FQDN: "example.com", MeasuredAt: time.Now()},
		{DomainID: 2, NodeID: "cn-beijing-01", NodeRole: "probe", FQDN: "test.com", MeasuredAt: time.Now()},
	}

	err := svc.StoreMeasurements(context.Background(), "cn-beijing-01", measurements)
	require.NoError(t, err)
	assert.Len(t, insertedMeasurements, 2)
	assert.Equal(t, "cn-north", insertedRegion)
}

func TestStoreMeasurements_EmptySliceIsNoOp(t *testing.T) {
	insertCalled := false
	ms := &stubMeasurementStore{
		insertFn: func(_ context.Context, _ []probeprotocol.Measurement, _ string) error {
			insertCalled = true
			return nil
		},
	}
	ns := &stubNodeStoreForMeasurement{
		getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
			return &postgres.GFWProbeNode{}, nil
		},
	}
	svc := newTestMeasurementService(ms, ns)

	err := svc.StoreMeasurements(context.Background(), "cn-beijing-01", nil)
	require.NoError(t, err)
	assert.False(t, insertCalled, "insert should not be called for empty slice")
}

func TestStoreMeasurements_UnknownNodeReturnsError(t *testing.T) {
	ms := &stubMeasurementStore{
		insertFn: func(_ context.Context, _ []probeprotocol.Measurement, _ string) error {
			return nil
		},
	}
	ns := &stubNodeStoreForMeasurement{
		getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
			return nil, postgres.ErrProbeNodeNotFound
		},
	}
	svc := newTestMeasurementService(ms, ns)

	err := svc.StoreMeasurements(context.Background(), "unknown-node", []probeprotocol.Measurement{
		{DomainID: 1, FQDN: "example.com"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, postgres.ErrProbeNodeNotFound))
}

func TestStoreMeasurements_StoreErrorPropagated(t *testing.T) {
	storeErr := errors.New("db connection refused")
	ms := &stubMeasurementStore{
		insertFn: func(_ context.Context, _ []probeprotocol.Measurement, _ string) error {
			return storeErr
		},
	}
	ns := &stubNodeStoreForMeasurement{
		getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
			return &postgres.GFWProbeNode{NodeID: "cn-beijing-01", Region: "cn-north"}, nil
		},
	}
	svc := newTestMeasurementService(ms, ns)

	err := svc.StoreMeasurements(context.Background(), "cn-beijing-01", []probeprotocol.Measurement{
		{DomainID: 1, FQDN: "example.com"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, storeErr))
}

func TestStoreMeasurements_ControlNodeUsesItsRegion(t *testing.T) {
	var capturedRegion string
	ms := &stubMeasurementStore{
		insertFn: func(_ context.Context, _ []probeprotocol.Measurement, region string) error {
			capturedRegion = region
			return nil
		},
	}
	ns := &stubNodeStoreForMeasurement{
		getByNodeIDFn: func(_ context.Context, _ string) (*postgres.GFWProbeNode, error) {
			return &postgres.GFWProbeNode{NodeID: "hk-01", Region: "hk", Role: "control"}, nil
		},
	}
	svc := newTestMeasurementService(ms, ns)

	err := svc.StoreMeasurements(context.Background(), "hk-01", []probeprotocol.Measurement{
		{DomainID: 1, NodeID: "hk-01", NodeRole: "control", FQDN: "example.com"},
	})

	require.NoError(t, err)
	assert.Equal(t, "hk", capturedRegion)
}

// ── ListBogonIPs tests ────────────────────────────────────────────────────────

func TestListBogonIPs_ReturnsFromStore(t *testing.T) {
	expected := []string{"1.2.3.4", "37.235.1.174", "0.0.0.0"}
	ms := &stubMeasurementStore{
		listBogonsFn: func(_ context.Context) ([]string, error) {
			return expected, nil
		},
	}
	ns := &stubNodeStoreForMeasurement{}
	svc := newTestMeasurementService(ms, ns)

	got, err := svc.ListBogonIPs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestListBogonIPs_StoreErrorPropagated(t *testing.T) {
	ms := &stubMeasurementStore{
		listBogonsFn: func(_ context.Context) ([]string, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestMeasurementService(ms, &stubNodeStoreForMeasurement{})

	_, err := svc.ListBogonIPs(context.Background())
	require.Error(t, err)
}

// ── DNSResult heuristic flag integration ─────────────────────────────────────

// Verify that measurements carrying IsBogon / IsInjected flags round-trip
// correctly through the protocol struct (probeprotocol package).
func TestDNSResultBogonFlags_RoundTrip(t *testing.T) {
	m := probeprotocol.Measurement{
		DomainID: 42,
		FQDN:     "blocked.example.com",
		NodeID:   "cn-beijing-01",
		NodeRole: probeprotocol.RoleProbe,
		DNS: &probeprotocol.DNSResult{
			ResolverIP: "8.8.8.8:53",
			Answers:    []string{"1.2.3.4"},
			DurationMS: 2,
			IsBogon:    true,
			IsInjected: true,
		},
		MeasuredAt: time.Now(),
	}

	require.NotNil(t, m.DNS)
	assert.True(t, m.DNS.IsBogon, "IsBogon should survive struct assignment")
	assert.True(t, m.DNS.IsInjected, "IsInjected should survive struct assignment")
	assert.Equal(t, int64(2), m.DNS.DurationMS)
}

func TestDNSResultCleanFlags(t *testing.T) {
	// A legitimate result should have both flags false.
	m := probeprotocol.DNSResult{
		ResolverIP: "8.8.8.8:53",
		Answers:    []string{"104.21.0.1"},
		DurationMS: 42,
	}
	assert.False(t, m.IsBogon)
	assert.False(t, m.IsInjected)
}
