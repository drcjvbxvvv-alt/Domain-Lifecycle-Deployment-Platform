package dnsquery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	dnsprovider "domain-platform/pkg/provider/dns"
	"domain-platform/store/postgres"
)

// ── Drift result types ───────────────────────────────────────────────────────

// DriftStatus summarises the drift state.
type DriftStatus string

const (
	DriftOK        DriftStatus = "ok"        // expected == actual
	DriftDetected  DriftStatus = "drift"     // differences found
	DriftNoExpected DriftStatus = "no_expected" // provider returned 0 records (or no provider configured)
	DriftError     DriftStatus = "error"     // provider API call failed
)

// DriftRecord pairs an expected record with its actual counterpart.
// Missing records have only one side populated.
type DriftRecord struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Expected string `json:"expected,omitempty"` // value from provider API
	Actual   string `json:"actual,omitempty"`   // value from live DNS
	Match    bool   `json:"match"`
}

// DriftResult is the full drift check response.
type DriftResult struct {
	FQDN             string        `json:"fqdn"`
	ProviderName     string        `json:"provider_name"`     // e.g. "cloudflare"
	ProviderLabel    string        `json:"provider_label"`    // e.g. "My Cloudflare"
	Status           DriftStatus   `json:"status"`
	Records          []DriftRecord `json:"records"`
	ExpectedCount    int           `json:"expected_count"`
	ActualCount      int           `json:"actual_count"`
	MatchCount       int           `json:"match_count"`
	DriftCount       int           `json:"drift_count"`
	MissingCount     int           `json:"missing_count"`      // in expected but not in actual
	ExtraCount       int           `json:"extra_count"`        // in actual but not in expected
	QueriedAt        string        `json:"queried_at"`
	ElapsedMs        int64         `json:"elapsed_ms"`
	Error            string        `json:"error,omitempty"`
}

// ── Drift check ──────────────────────────────────────────────────────────────

// CheckDrift compares DNS records from the provider API ("expected") against
// live DNS resolution ("actual") for the given domain.
//
// It requires the domain to have a dns_provider_id set, and the provider must
// have valid credentials configured. Returns DriftNoExpected if no provider.
func (s *Service) CheckDrift(
	ctx context.Context,
	domain *postgres.Domain,
	provider *postgres.DNSProvider,
) *DriftResult {
	start := time.Now()
	result := &DriftResult{
		FQDN:      domain.FQDN,
		QueriedAt: now(),
	}

	if provider == nil {
		result.Status = DriftNoExpected
		result.Error = "domain has no DNS provider configured"
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}

	result.ProviderName = provider.ProviderType
	result.ProviderLabel = provider.Name

	// Get provider instance
	p, err := dnsprovider.Get(provider.ProviderType, provider.Config, provider.Credentials)
	if err != nil {
		result.Status = DriftError
		result.Error = fmt.Sprintf("init provider: %v", err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}

	// Extract zone_id from config
	zoneID := extractZoneID(provider.Config)

	// Fetch expected records from provider API
	expected, err := p.ListRecords(ctx, zoneID, dnsprovider.RecordFilter{Name: domain.FQDN})
	if err != nil {
		result.Status = DriftError
		result.Error = fmt.Sprintf("list records from provider: %v", err)
		result.ElapsedMs = time.Since(start).Milliseconds()
		return result
	}

	// Fetch actual records from live DNS
	lookupResult := s.Lookup(ctx, domain.FQDN)
	actual := lookupResult.Records

	// Compare
	driftRecords := CompareRecords(expected, actual)
	result.Records = driftRecords
	result.ExpectedCount = len(expected)
	result.ActualCount = len(actual)

	for _, dr := range driftRecords {
		if dr.Match {
			result.MatchCount++
		} else if dr.Expected != "" && dr.Actual == "" {
			result.MissingCount++
		} else if dr.Expected == "" && dr.Actual != "" {
			result.ExtraCount++
		} else {
			result.DriftCount++
		}
	}

	if result.MissingCount == 0 && result.DriftCount == 0 && result.ExtraCount == 0 {
		result.Status = DriftOK
	} else if len(expected) == 0 {
		result.Status = DriftNoExpected
	} else {
		result.Status = DriftDetected
	}

	result.ElapsedMs = time.Since(start).Milliseconds()

	s.logger.Info("drift check completed",
		zap.String("fqdn", domain.FQDN),
		zap.String("status", string(result.Status)),
		zap.Int("expected", result.ExpectedCount),
		zap.Int("actual", result.ActualCount),
		zap.Int("match", result.MatchCount),
		zap.Int("drift", result.DriftCount),
		zap.Int("missing", result.MissingCount),
		zap.Int("extra", result.ExtraCount),
	)

	return result
}

// ── Comparison logic (pure, no I/O) ──────────────────────────────────────────

// CompareRecords compares expected (provider API) records against actual
// (live DNS) records. Returns a list of DriftRecord with match status.
//
// Matching is by (type, normalised value). TTL and provider-specific fields
// (e.g. Proxied) are intentionally ignored.
func CompareRecords(expected []dnsprovider.Record, actual []Record) []DriftRecord {
	// Build lookup maps: key = "TYPE|value"
	expectedMap := make(map[string]dnsprovider.Record)
	for _, r := range expected {
		key := normaliseKey(r.Type, r.Content)
		expectedMap[key] = r
	}

	actualMap := make(map[string]Record)
	for _, r := range actual {
		key := normaliseKey(string(r.Type), r.Value)
		actualMap[key] = r
	}

	var result []DriftRecord
	seen := make(map[string]bool)

	// Walk expected: match or missing
	for key, exp := range expectedMap {
		seen[key] = true
		act, found := actualMap[key]
		if found {
			result = append(result, DriftRecord{
				Type:     exp.Type,
				Name:     exp.Name,
				Expected: exp.Content,
				Actual:   act.Value,
				Match:    true,
			})
		} else {
			result = append(result, DriftRecord{
				Type:     exp.Type,
				Name:     exp.Name,
				Expected: exp.Content,
				Actual:   "",
				Match:    false,
			})
		}
	}

	// Walk actual: find extras (not in expected)
	for key, act := range actualMap {
		if seen[key] {
			continue
		}
		result = append(result, DriftRecord{
			Type:     string(act.Type),
			Name:     act.Name,
			Expected: "",
			Actual:   act.Value,
			Match:    false,
		})
	}

	// Sort: type, then name, then expected/actual
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		ei := result[i].Expected + result[i].Actual
		ej := result[j].Expected + result[j].Actual
		return ei < ej
	})

	return result
}

// normaliseKey builds a dedup key from record type + value.
// Trims trailing dots and lowercases for consistent comparison.
func normaliseKey(rtype, value string) string {
	return strings.ToUpper(rtype) + "|" + strings.ToLower(strings.TrimSuffix(value, "."))
}

// extractZoneID pulls zone_id from provider config JSONB.
func extractZoneID(config json.RawMessage) string {
	var c struct {
		ZoneID string `json:"zone_id"`
	}
	_ = json.Unmarshal(config, &c)
	return c.ZoneID
}
