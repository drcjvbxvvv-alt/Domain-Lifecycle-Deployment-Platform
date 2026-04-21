// Package domain contains domain-level business logic that doesn't belong in
// the lifecycle state machine (which is strictly about state transitions).
package domain

import (
	"context"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	"domain-platform/store/postgres"
)

// Expiry status constants — must match the domains.expiry_status CHECK constraint.
const (
	StatusExpiring90d = "expiring_90d"
	StatusExpiring30d = "expiring_30d"
	StatusExpiring7d  = "expiring_7d"
	StatusExpired     = "expired"
	StatusGrace       = "grace"
)

// Severity for notification grouping.
const (
	SeverityInfo     = "info"     // 90d
	SeverityWarning  = "warning"  // 30d
	SeverityUrgent   = "urgent"   // 7d
	SeverityCritical = "critical" // expired / grace
)

// ComputeExpiryStatus is a pure function that computes the expiry_status for a
// domain based on its expiry date, optional grace-end date, and the current time.
//
// Returns nil when the domain has no expiry concern (more than 90 days away or
// no expiry date). A nil return maps to SQL NULL, meaning "ok".
func ComputeExpiryStatus(expiryDate *time.Time, graceEndDate *time.Time, now time.Time) *string {
	if expiryDate == nil {
		return nil
	}

	// Domain has already passed its expiry date
	if !now.Before(*expiryDate) {
		if graceEndDate != nil && now.Before(*graceEndDate) {
			return strPtr(StatusGrace)
		}
		return strPtr(StatusExpired)
	}

	daysLeft := daysUntil(*expiryDate, now)

	if daysLeft <= 7 {
		return strPtr(StatusExpiring7d)
	}
	if daysLeft <= 30 {
		return strPtr(StatusExpiring30d)
	}
	if daysLeft <= 90 {
		return strPtr(StatusExpiring90d)
	}
	return nil // ok — no special status
}

// SeverityForStatus maps an expiry_status value to a notification severity.
func SeverityForStatus(status *string) string {
	if status == nil {
		return ""
	}
	switch *status {
	case StatusExpiring90d:
		return SeverityInfo
	case StatusExpiring30d:
		return SeverityWarning
	case StatusExpiring7d:
		return SeverityUrgent
	case StatusExpired, StatusGrace:
		return SeverityCritical
	default:
		return SeverityInfo
	}
}

// ExpiryStateChange records one domain whose expiry_status has changed.
type ExpiryStateChange struct {
	DomainID   int64
	FQDN       string
	ExpiryDate time.Time
	OldStatus  *string
	NewStatus  *string
}

// ExpiryCheckResult is the return value of CheckAllExpiry.
type ExpiryCheckResult struct {
	Checked int
	Changed []ExpiryStateChange
}

// ExpiryService runs batch expiry checks across all relevant domains.
type ExpiryService struct {
	domainStore *postgres.DomainStore
	logger      *zap.Logger
}

func NewExpiryService(domainStore *postgres.DomainStore, logger *zap.Logger) *ExpiryService {
	return &ExpiryService{domainStore: domainStore, logger: logger}
}

// CheckAllExpiry iterates every non-retired domain with an expiry_date,
// computes the new expiry_status, persists changes, and returns all state changes.
// Idempotent: running twice on the same day produces zero changes on the second run.
func (s *ExpiryService) CheckAllExpiry(ctx context.Context) (*ExpiryCheckResult, error) {
	now := time.Now()

	// Get all non-retired domains with an expiry date.
	// We use a broad filter and post-filter in Go to avoid building complex SQL.
	domains, err := s.domainStore.ListWithFilter(ctx, postgres.ListFilter{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("list domains for expiry check: %w", err)
	}

	result := &ExpiryCheckResult{}

	for _, d := range domains {
		if d.LifecycleState == "retired" || d.ExpiryDate == nil {
			continue
		}
		result.Checked++

		newStatus := ComputeExpiryStatus(d.ExpiryDate, d.GraceEndDate, now)

		// Compare old vs new — detect change.
		if statusEqual(d.ExpiryStatus, newStatus) {
			continue
		}

		// Status changed — persist and record.
		if err := s.domainStore.UpdateExpiryStatus(ctx, d.ID, newStatus); err != nil {
			s.logger.Warn("update expiry status failed",
				zap.Int64("domain_id", d.ID),
				zap.String("fqdn", d.FQDN),
				zap.Error(err),
			)
			continue
		}

		change := ExpiryStateChange{
			DomainID:  d.ID,
			FQDN:      d.FQDN,
			OldStatus: d.ExpiryStatus,
			NewStatus: newStatus,
		}
		if d.ExpiryDate != nil {
			change.ExpiryDate = *d.ExpiryDate
		}
		result.Changed = append(result.Changed, change)
	}

	s.logger.Info("expiry check complete",
		zap.Int("checked", result.Checked),
		zap.Int("changed", len(result.Changed)),
	)
	return result, nil
}

// GetExpiryDashboardData returns counts of domains per expiry band for the dashboard.
type ExpiryBand struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type CalendarEntry struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Count int    `json:"count"`
}

type DashboardData struct {
	DomainBands []ExpiryBand    `json:"domain_bands"`
	TotalExpiry int             `json:"total_expiring"`
	Calendar    []CalendarEntry `json:"calendar"` // next 90 days
}

func (s *ExpiryService) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	now := time.Now()

	domains, err := s.domainStore.ListWithFilter(ctx, postgres.ListFilter{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("list domains for dashboard: %w", err)
	}

	bandCounts := map[string]int{
		StatusExpired:     0,
		StatusGrace:       0,
		StatusExpiring7d:  0,
		StatusExpiring30d: 0,
		StatusExpiring90d: 0,
	}
	calMap := make(map[string]int) // date → count
	totalExpiring := 0

	for _, d := range domains {
		if d.LifecycleState == "retired" || d.ExpiryDate == nil {
			continue
		}
		status := ComputeExpiryStatus(d.ExpiryDate, d.GraceEndDate, now)
		if status != nil {
			bandCounts[*status]++
			totalExpiring++
		}

		// Calendar: only show domains expiring in next 90 days
		daysLeft := daysUntil(*d.ExpiryDate, now)
		if daysLeft >= 0 && daysLeft <= 90 {
			dateStr := d.ExpiryDate.Format("2006-01-02")
			calMap[dateStr]++
		}
	}

	bands := make([]ExpiryBand, 0, len(bandCounts))
	for _, s := range []string{StatusExpired, StatusGrace, StatusExpiring7d, StatusExpiring30d, StatusExpiring90d} {
		bands = append(bands, ExpiryBand{Status: s, Count: bandCounts[s]})
	}

	calendar := make([]CalendarEntry, 0, len(calMap))
	for date, count := range calMap {
		calendar = append(calendar, CalendarEntry{Date: date, Count: count})
	}

	return &DashboardData{
		DomainBands: bands,
		TotalExpiry: totalExpiring,
		Calendar:    calendar,
	}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func daysUntil(target, now time.Time) int {
	return int(math.Ceil(target.Sub(now).Hours() / 24))
}

func statusEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtr(s string) *string { return &s }
