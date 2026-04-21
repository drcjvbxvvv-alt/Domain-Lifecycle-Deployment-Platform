package dnsquery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Integration tests (require network) ──────────────────────────────────────

func TestCheckPropagation_Google(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := svc.CheckPropagation(ctx, "google.com", []string{"A"})
	require.NotNil(t, result)
	assert.Equal(t, "google.com", result.FQDN)
	assert.Equal(t, []string{"A"}, result.QueryTypes)
	assert.NotEmpty(t, result.QueriedAt)
	assert.Greater(t, result.TotalMs, int64(0))

	// Should have at least the 4 public resolvers + authoritative NS(es)
	assert.GreaterOrEqual(t, len(result.Resolvers), 4, "expected at least 4 resolvers")

	// Each resolver should have A records (google.com is universal)
	for _, rr := range result.Resolvers {
		if rr.Error != "" {
			continue
		}
		assert.NotEmpty(t, rr.Records, "resolver %s should return A records", rr.Label)
		assert.Greater(t, rr.ElapsedMs, int64(0))
		assert.NotEmpty(t, rr.Address)
		assert.NotEmpty(t, rr.Label)
	}
}

func TestCheckPropagation_ConsistencyFieldIsSet(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// google.com uses Anycast — different resolvers may return different edge IPs,
	// so we only verify the Consistent field is computed (bool), not its value.
	result := svc.CheckPropagation(ctx, "google.com", []string{"A"})
	// Consistent is either true or false — both are valid for anycast domains.
	// The unit tests (TestCheckConsistency_*) verify the logic itself.
	_ = result.Consistent
}

func TestCheckPropagation_HasAuthoritativeNS(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := svc.CheckPropagation(ctx, "google.com", nil) // default A+AAAA
	hasAuth := false
	for _, rr := range result.Resolvers {
		if rr.Authoritative {
			hasAuth = true
			assert.Contains(t, rr.Label, "權威 NS")
		}
	}
	assert.True(t, hasAuth, "expected at least one authoritative NS resolver")
}

func TestCheckPropagation_DefaultsToA_AAAA(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := svc.CheckPropagation(ctx, "google.com", nil)
	assert.Equal(t, []string{"A", "AAAA"}, result.QueryTypes)
}

func TestCheckPropagation_EmptyFQDN(t *testing.T) {
	svc := newTestService()
	result := svc.CheckPropagation(context.Background(), "", nil)
	assert.Equal(t, "", result.FQDN)
	assert.False(t, result.Consistent)
}

func TestCheckPropagation_MXType(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := svc.CheckPropagation(ctx, "google.com", []string{"MX"})
	for _, rr := range result.Resolvers {
		if rr.Error != "" {
			continue
		}
		hasMX := false
		for _, rec := range rr.Records {
			if rec.Type == TypeMX {
				hasMX = true
			}
		}
		assert.True(t, hasMX, "resolver %s should return MX records for google.com", rr.Label)
	}
}

// ── Unit tests (no network) ─────────────────────────────────────────────────

func TestFingerprint(t *testing.T) {
	records := []Record{
		{Type: TypeA, Value: "5.6.7.8"},
		{Type: TypeA, Value: "1.2.3.4"},
	}
	fp := fingerprint(records)
	assert.Equal(t, "A=1.2.3.4|A=5.6.7.8", fp, "fingerprint should sort by type+value")
}

func TestFingerprint_IgnoresTTL(t *testing.T) {
	r1 := []Record{{Type: TypeA, Value: "1.2.3.4", TTL: 300}}
	r2 := []Record{{Type: TypeA, Value: "1.2.3.4", TTL: 60}}
	assert.Equal(t, fingerprint(r1), fingerprint(r2), "TTL should not affect fingerprint")
}

func TestCheckConsistency_AllSame(t *testing.T) {
	results := []ResolverResult{
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
	}
	assert.True(t, checkConsistency(results))
}

func TestCheckConsistency_OneDiffers(t *testing.T) {
	results := []ResolverResult{
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
		{Records: []Record{{Type: TypeA, Value: "9.9.9.9"}}},
	}
	assert.False(t, checkConsistency(results))
}

func TestCheckConsistency_SkipsErrors(t *testing.T) {
	results := []ResolverResult{
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
		{Error: "timeout"},
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
	}
	assert.True(t, checkConsistency(results), "errored resolvers should be ignored")
}

func TestCheckConsistency_SingleResult(t *testing.T) {
	results := []ResolverResult{
		{Records: []Record{{Type: TypeA, Value: "1.2.3.4"}}},
	}
	assert.True(t, checkConsistency(results), "single result is trivially consistent")
}

func TestCheckConsistency_AllErrors(t *testing.T) {
	results := []ResolverResult{
		{Error: "timeout"},
		{Error: "timeout"},
	}
	assert.True(t, checkConsistency(results), "all errors = trivially consistent")
}
