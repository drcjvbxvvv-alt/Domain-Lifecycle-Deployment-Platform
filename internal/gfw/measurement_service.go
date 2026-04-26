package gfw

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"domain-platform/cmd/probe/checker"
	"domain-platform/pkg/probeprotocol"
	"domain-platform/store/postgres"
)

// MeasurementService handles ingestion and retrieval of raw GFW measurements
// reported by probe and control nodes.
type MeasurementService struct {
	mstore *postgres.GFWMeasurementStore
	nstore *postgres.GFWNodeStore
	logger *zap.Logger
}

// NewMeasurementService creates a MeasurementService.
func NewMeasurementService(
	mstore *postgres.GFWMeasurementStore,
	nstore *postgres.GFWNodeStore,
	logger *zap.Logger,
) *MeasurementService {
	return &MeasurementService{
		mstore: mstore,
		nstore: nstore,
		logger: logger,
	}
}

// StoreMeasurements validates that nodeID is registered, then bulk-inserts
// measurements into gfw_measurements.
// Returns an error only for infrastructure failures; individual measurement
// encoding errors are logged and skipped.
func (s *MeasurementService) StoreMeasurements(
	ctx context.Context,
	nodeID string,
	measurements []probeprotocol.Measurement,
) error {
	if len(measurements) == 0 {
		return nil
	}

	// Validate that the submitting node is known to us.
	node, err := s.nstore.GetByNodeID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("validate probe node %q: %w", nodeID, err)
	}

	region := node.Region

	if err := s.mstore.InsertMeasurements(ctx, measurements, region); err != nil {
		return fmt.Errorf("insert measurements for node %q: %w", nodeID, err)
	}

	s.logger.Info("measurements stored",
		zap.String("node_id", nodeID),
		zap.String("role", node.Role),
		zap.String("region", region),
		zap.Int("count", len(measurements)),
	)
	return nil
}

// ListMeasurements returns measurements for a domain within an optional time
// window.  Pass zero Time values to skip the bound.
func (s *MeasurementService) ListMeasurements(
	ctx context.Context,
	domainID int64,
	from, to time.Time,
	limit int,
) ([]postgres.GFWMeasurement, error) {
	rows, err := s.mstore.ListMeasurements(ctx, domainID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("list measurements domain=%d: %w", domainID, err)
	}
	return rows, nil
}

// GetLatestMeasurements returns the most recent probe measurement and the most
// recent control measurement for domainID.
// Either return value may be nil when no measurement of that role exists.
func (s *MeasurementService) GetLatestMeasurements(
	ctx context.Context,
	domainID int64,
) (probe *postgres.GFWMeasurement, control *postgres.GFWMeasurement, err error) {
	p, c, err := s.mstore.LatestMeasurementPair(ctx, domainID)
	if err != nil {
		return nil, nil, fmt.Errorf("get latest measurements domain=%d: %w", domainID, err)
	}
	return p, c, nil
}

// ListBogonIPs returns the full bogon IP list loaded from the database.
// Used by probe nodes to refresh their local BogonList at startup.
func (s *MeasurementService) ListBogonIPs(ctx context.Context) ([]string, error) {
	ips, err := s.mstore.ListBogonIPs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bogon ips: %w", err)
	}
	return ips, nil
}

// BuildBogonList loads the bogon list from the DB and constructs a
// checker.BogonList.  Falls back to the default (hardcoded) list on DB error.
func (s *MeasurementService) BuildBogonList(ctx context.Context) checker.BogonList {
	ips, err := s.mstore.ListBogonIPs(ctx)
	if err != nil {
		s.logger.Warn("failed to load bogon IPs from DB — using built-in defaults",
			zap.Error(err))
		return checker.DefaultBogonList()
	}
	if len(ips) == 0 {
		return checker.DefaultBogonList()
	}
	return checker.NewBogonList(ips)
}

// AddBogonIP adds an operator-defined bogon IP.
func (s *MeasurementService) AddBogonIP(ctx context.Context, ip, note string) error {
	if ip == "" {
		return fmt.Errorf("ip_address is required")
	}
	if err := s.mstore.InsertBogonIP(ctx, ip, note); err != nil {
		return fmt.Errorf("add bogon ip %q: %w", ip, err)
	}
	s.logger.Info("operator bogon IP added", zap.String("ip", ip))
	return nil
}

// DeleteBogonIP removes an operator-defined bogon IP.
// Seeded IPs cannot be removed.
func (s *MeasurementService) DeleteBogonIP(ctx context.Context, ip string) error {
	if err := s.mstore.DeleteBogonIP(ctx, ip); err != nil {
		return fmt.Errorf("delete bogon ip %q: %w", ip, err)
	}
	s.logger.Info("operator bogon IP removed", zap.String("ip", ip))
	return nil
}
