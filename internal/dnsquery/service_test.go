package dnsquery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestService() *Service {
	logger, _ := zap.NewDevelopment()
	return NewService(logger)
}

// TestLookup_Google tests a real DNS lookup against google.com.
// This is an integration test — it requires network. Skip in CI if needed.
func TestLookup_Google(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result := svc.Lookup(ctx, "google.com")
	require.NotNil(t, result)
	assert.Equal(t, "google.com", result.FQDN)
	assert.Empty(t, result.Error)
	assert.NotEmpty(t, result.QueriedAt)

	// google.com should have at least A records and NS records
	hasA := false
	hasNS := false
	for _, r := range result.Records {
		switch r.Type {
		case TypeA:
			hasA = true
			assert.NotEmpty(t, r.Value)
		case TypeNS:
			hasNS = true
			assert.NotEmpty(t, r.Value)
		}
	}
	assert.True(t, hasA, "expected at least one A record for google.com")
	assert.True(t, hasNS, "expected at least one NS record for google.com")
}

// TestLookup_MXRecords verifies MX records for a well-known mail domain.
func TestLookup_MXRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result := svc.Lookup(ctx, "google.com")
	hasMX := false
	for _, r := range result.Records {
		if r.Type == TypeMX {
			hasMX = true
			assert.Greater(t, r.Priority, 0)
			assert.NotEmpty(t, r.Value)
		}
	}
	assert.True(t, hasMX, "expected MX records for google.com")
}

// TestLookup_EmptyFQDN returns an error result.
func TestLookup_EmptyFQDN(t *testing.T) {
	svc := newTestService()
	result := svc.Lookup(context.Background(), "")
	assert.Equal(t, "empty FQDN", result.Error)
	assert.Empty(t, result.Records)
}

// TestLookup_NonexistentDomain returns empty records, no crash.
func TestLookup_NonexistentDomain(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := svc.Lookup(ctx, "this-domain-definitely-does-not-exist-abc123xyz.com")
	// Should not crash — records will be empty
	assert.NotNil(t, result)
	assert.Empty(t, result.Error) // partial failures don't set top-level error
}

// TestLookup_TrailingDotStripped ensures trailing dots are handled.
func TestLookup_TrailingDotStripped(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := svc.Lookup(ctx, "google.com.")
	assert.Equal(t, "google.com", result.FQDN)
}

// TestLookup_RecordsSorted verifies A < AAAA < CNAME < MX < NS < TXT ordering.
func TestLookup_RecordsSorted(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result := svc.Lookup(ctx, "google.com")
	if len(result.Records) < 2 {
		t.Skip("not enough records to test sorting")
	}

	typeOrder := map[RecordType]int{TypeA: 0, TypeAAAA: 1, TypeCNAME: 2, TypeMX: 3, TypeNS: 4, TypeTXT: 5}
	for i := 1; i < len(result.Records); i++ {
		prev := typeOrder[result.Records[i-1].Type]
		curr := typeOrder[result.Records[i].Type]
		assert.LessOrEqual(t, prev, curr, "records should be sorted by type order")
	}
}

// TestLookupMultiple_Concurrent tests batch lookup.
func TestLookupMultiple_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skip network test in short mode")
	}
	svc := newTestService()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fqdns := []string{"google.com", "cloudflare.com", "github.com"}
	results := svc.LookupMultiple(ctx, fqdns)
	require.Len(t, results, 3)

	for i, r := range results {
		assert.Equal(t, fqdns[i], r.FQDN)
		assert.NotEmpty(t, r.QueriedAt)
	}
}
