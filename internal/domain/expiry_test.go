package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ── ComputeExpiryStatus ──────────────────────────────────────────────────────

func TestComputeExpiryStatus(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	d := func(days int) *time.Time {
		t := now.AddDate(0, 0, days)
		return &t
	}

	tests := []struct {
		name         string
		expiryDate   *time.Time
		graceEndDate *time.Time
		want         *string
	}{
		// nil expiry → nil (ok)
		{"nil expiry", nil, nil, nil},

		// ok: well into the future
		{"365 days out", d(365), nil, nil},
		{"91 days out", d(91), nil, nil},

		// expiring_90d band: (30, 90]
		{"exactly 90 days", d(90), nil, strPtr(StatusExpiring90d)},
		{"89 days", d(89), nil, strPtr(StatusExpiring90d)},
		{"31 days", d(31), nil, strPtr(StatusExpiring90d)},

		// expiring_30d band: (7, 30]
		{"exactly 30 days", d(30), nil, strPtr(StatusExpiring30d)},
		{"29 days", d(29), nil, strPtr(StatusExpiring30d)},
		{"8 days", d(8), nil, strPtr(StatusExpiring30d)},

		// expiring_7d band: (0, 7]
		{"exactly 7 days", d(7), nil, strPtr(StatusExpiring7d)},
		{"6 days", d(6), nil, strPtr(StatusExpiring7d)},
		{"1 day", d(1), nil, strPtr(StatusExpiring7d)},

		// expired: past expiry, no grace
		{"expired today (exact)", d(0), nil, strPtr(StatusExpired)},
		{"expired 1 day ago", d(-1), nil, strPtr(StatusExpired)},
		{"expired 30 days ago", d(-30), nil, strPtr(StatusExpired)},

		// grace: past expiry, within grace window
		{"in grace period", d(-2), d(28), strPtr(StatusGrace)},
		{"grace starts at expiry", d(0), d(30), strPtr(StatusGrace)},
		{"grace last day", d(-29), d(1), strPtr(StatusGrace)},

		// grace expired
		{"grace period ended", d(-40), d(-10), strPtr(StatusExpired)},

		// PA.7 acceptance criteria
		{"acceptance: +25 days → expiring_30d", d(25), nil, strPtr(StatusExpiring30d)},
		{"acceptance: -2 days → expired", d(-2), nil, strPtr(StatusExpired)},
		{"acceptance: -2 days + grace +28 → grace", d(-2), d(28), strPtr(StatusGrace)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeExpiryStatus(tt.expiryDate, tt.graceEndDate, now)
			if tt.want == nil {
				assert.Nil(t, got, "expected nil (ok), got %v", got)
			} else {
				if assert.NotNil(t, got, "expected %q, got nil", *tt.want) {
					assert.Equal(t, *tt.want, *got)
				}
			}
		})
	}
}

// ── SeverityForStatus ─────────────────────────────────────────────────────────

func TestSeverityForStatus(t *testing.T) {
	tests := []struct {
		status *string
		want   string
	}{
		{nil, ""},
		{strPtr(StatusExpiring90d), SeverityInfo},
		{strPtr(StatusExpiring30d), SeverityWarning},
		{strPtr(StatusExpiring7d), SeverityUrgent},
		{strPtr(StatusExpired), SeverityCritical},
		{strPtr(StatusGrace), SeverityCritical},
	}
	for _, tt := range tests {
		name := "nil"
		if tt.status != nil {
			name = *tt.status
		}
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, SeverityForStatus(tt.status))
		})
	}
}

// ── statusEqual ───────────────────────────────────────────────────────────────

func TestStatusEqual(t *testing.T) {
	assert.True(t, statusEqual(nil, nil))
	assert.False(t, statusEqual(nil, strPtr("x")))
	assert.False(t, statusEqual(strPtr("x"), nil))
	assert.True(t, statusEqual(strPtr("expired"), strPtr("expired")))
	assert.False(t, statusEqual(strPtr("expired"), strPtr("grace")))
}

// ── daysUntil ─────────────────────────────────────────────────────────────────

func TestDaysUntil(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	// Exactly 1 day ahead (24h)
	assert.Equal(t, 1, daysUntil(now.Add(24*time.Hour), now))

	// 12 hours ahead → ceil to 1
	assert.Equal(t, 1, daysUntil(now.Add(12*time.Hour), now))

	// Exactly 0 hours (same instant)
	assert.Equal(t, 0, daysUntil(now, now))

	// 1 hour behind → ceil to 0 (negative partial day)
	assert.Equal(t, 0, daysUntil(now.Add(-time.Hour), now))

	// 30 days
	assert.Equal(t, 30, daysUntil(now.AddDate(0, 0, 30), now))
}

// ── Boundary tests ────────────────────────────────────────────────────────────

func TestComputeExpiryStatusBoundary(t *testing.T) {
	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)

	// Expiry at midnight on the same day → daysUntil = 0 → expired
	sameDay := now
	got := ComputeExpiryStatus(&sameDay, nil, now)
	assert.NotNil(t, got)
	assert.Equal(t, StatusExpired, *got)

	// Expiry 1 second in the future → daysUntil = ceil(1s/24h) = 1 → expiring_7d
	oneSecLater := now.Add(time.Second)
	got = ComputeExpiryStatus(&oneSecLater, nil, now)
	assert.NotNil(t, got)
	assert.Equal(t, StatusExpiring7d, *got)

	// Grace period: expired 1s ago, grace ends in 1 day
	oneSecAgo := now.Add(-time.Second)
	graceEnd := now.AddDate(0, 0, 1)
	got = ComputeExpiryStatus(&oneSecAgo, &graceEnd, now)
	assert.NotNil(t, got)
	assert.Equal(t, StatusGrace, *got)

	// Grace period: expired and grace also expired
	graceExpired := now.Add(-time.Second)
	got = ComputeExpiryStatus(&oneSecAgo, &graceExpired, now)
	assert.NotNil(t, got)
	assert.Equal(t, StatusExpired, *got)
}
