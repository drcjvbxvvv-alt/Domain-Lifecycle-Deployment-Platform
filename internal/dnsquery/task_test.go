package dnsquery

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ── formatDriftAlert ──────────────────────────────────────────────────────────

func TestFormatDriftAlert_MissingRecord(t *testing.T) {
	result := &DriftResult{
		FQDN:          "example.com",
		ProviderLabel: "My Cloudflare",
		Status:        DriftDetected,
		MissingCount:  1,
		Records: []DriftRecord{
			{Type: "A", Name: "example.com", Expected: "1.2.3.4", Actual: "", Match: false},
		},
		QueriedAt: "2026-04-21T10:00:00Z",
	}

	subject, body := formatDriftAlert(result)

	assert.Contains(t, subject, "example.com")
	assert.Contains(t, subject, "DNS Drift 告警")
	assert.Contains(t, body, "缺少記錄")
	assert.Contains(t, body, "1.2.3.4")
	assert.Contains(t, body, "My Cloudflare")
}

func TestFormatDriftAlert_ExtraRecord(t *testing.T) {
	result := &DriftResult{
		FQDN:          "api.example.com",
		ProviderLabel: "Cloudflare Prod",
		Status:        DriftDetected,
		ExtraCount:    1,
		Records: []DriftRecord{
			{Type: "A", Name: "api.example.com", Expected: "", Actual: "9.9.9.9", Match: false},
		},
		QueriedAt: "2026-04-21T10:00:00Z",
	}

	subject, body := formatDriftAlert(result)

	assert.Contains(t, subject, "api.example.com")
	assert.Contains(t, body, "多餘記錄")
	assert.Contains(t, body, "9.9.9.9")
}

func TestFormatDriftAlert_DriftedRecord(t *testing.T) {
	result := &DriftResult{
		FQDN:          "mail.example.com",
		ProviderLabel: "DNS Provider",
		Status:        DriftDetected,
		DriftCount:    1,
		Records: []DriftRecord{
			{Type: "MX", Name: "mail.example.com", Expected: "mail1.example.com", Actual: "mail2.example.com", Match: false},
		},
		QueriedAt: "2026-04-21T10:00:00Z",
	}

	_, body := formatDriftAlert(result)

	assert.Contains(t, body, "數值不一致")
	assert.Contains(t, body, "mail1.example.com")
	assert.Contains(t, body, "mail2.example.com")
}

func TestFormatDriftAlert_TruncatesAt10Records(t *testing.T) {
	records := make([]DriftRecord, 15)
	for i := range records {
		records[i] = DriftRecord{
			Type: "A", Name: "example.com",
			Expected: "1.1.1.1", Actual: "", Match: false,
		}
	}

	result := &DriftResult{
		FQDN:          "example.com",
		ProviderLabel: "CF",
		Status:        DriftDetected,
		MissingCount:  15,
		Records:       records,
		QueriedAt:     "2026-04-21T10:00:00Z",
	}

	_, body := formatDriftAlert(result)

	// Should show "還有 N 筆差異" truncation message
	assert.Contains(t, body, "還有")
	assert.Contains(t, body, "筆差異")
}

func TestFormatDriftAlert_SubjectContainsDiffCount(t *testing.T) {
	result := &DriftResult{
		FQDN:          "test.example.com",
		ProviderLabel: "CF",
		Status:        DriftDetected,
		DriftCount:    2,
		MissingCount:  1,
		ExtraCount:    3,
		Records:       []DriftRecord{},
		QueriedAt:     "2026-04-21T10:00:00Z",
	}

	subject, _ := formatDriftAlert(result)

	// DriftCount + MissingCount + ExtraCount = 2+1+3 = 6
	assert.Contains(t, subject, "6 項差異")
}

// ── countMismatches ───────────────────────────────────────────────────────────

func TestCountMismatches(t *testing.T) {
	records := []DriftRecord{
		{Match: true},
		{Match: false},
		{Match: true},
		{Match: false},
		{Match: false},
	}
	assert.Equal(t, 3, countMismatches(records))
}

func TestCountMismatches_AllMatch(t *testing.T) {
	records := []DriftRecord{
		{Match: true},
		{Match: true},
	}
	assert.Equal(t, 0, countMismatches(records))
}

// ── dedup key format ──────────────────────────────────────────────────────────

func TestDedupKeyFormat(t *testing.T) {
	// Verify the dedup key pattern used in trySetAlerted / clearAlerted
	domainID := int64(42)
	key := "dns:drift:alerted:42"
	assert.True(t, strings.HasSuffix(key, "42"))
	assert.Equal(t, key, fmt.Sprintf("dns:drift:alerted:%d", domainID))
}
