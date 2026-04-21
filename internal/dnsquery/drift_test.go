package dnsquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dnsprovider "domain-platform/pkg/provider/dns"
)

// ── CompareRecords unit tests ────────────────────────────────────────────────

func TestCompareRecords_AllMatch(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
		{Type: "AAAA", Name: "example.com", Content: "::1"},
		{Type: "MX", Name: "example.com", Content: "mail.example.com", Priority: 10},
	}
	actual := []Record{
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
		{Type: TypeAAAA, Name: "example.com", Value: "::1"},
		{Type: TypeMX, Name: "example.com", Value: "mail.example.com", Priority: 10},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 3)
	for _, r := range result {
		assert.True(t, r.Match, "record %s %s should match", r.Type, r.Expected)
	}
}

func TestCompareRecords_MissingInActual(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
		{Type: "A", Name: "example.com", Content: "5.6.7.8"},
	}
	actual := []Record{
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 2)

	matched := 0
	missing := 0
	for _, r := range result {
		if r.Match {
			matched++
		} else if r.Expected != "" && r.Actual == "" {
			missing++
			assert.Equal(t, "5.6.7.8", r.Expected)
		}
	}
	assert.Equal(t, 1, matched)
	assert.Equal(t, 1, missing)
}

func TestCompareRecords_ExtraInActual(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
	}
	actual := []Record{
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
		{Type: TypeA, Name: "example.com", Value: "9.9.9.9"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 2)

	extra := 0
	for _, r := range result {
		if !r.Match && r.Expected == "" && r.Actual != "" {
			extra++
			assert.Equal(t, "9.9.9.9", r.Actual)
		}
	}
	assert.Equal(t, 1, extra)
}

func TestCompareRecords_Empty(t *testing.T) {
	result := CompareRecords(nil, nil)
	assert.Empty(t, result)
}

func TestCompareRecords_NoExpected(t *testing.T) {
	actual := []Record{
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
	}
	result := CompareRecords(nil, actual)
	require.Len(t, result, 1)
	assert.False(t, result[0].Match)
	assert.Empty(t, result[0].Expected)
	assert.Equal(t, "1.2.3.4", result[0].Actual)
}

func TestCompareRecords_NoActual(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
	}
	result := CompareRecords(expected, nil)
	require.Len(t, result, 1)
	assert.False(t, result[0].Match)
	assert.Equal(t, "1.2.3.4", result[0].Expected)
	assert.Empty(t, result[0].Actual)
}

func TestCompareRecords_TrailingDotNormalised(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "CNAME", Name: "www.example.com", Content: "cdn.example.com."},
	}
	actual := []Record{
		{Type: TypeCNAME, Name: "www.example.com", Value: "cdn.example.com"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 1)
	assert.True(t, result[0].Match, "trailing dot should be normalised for comparison")
}

func TestCompareRecords_CaseInsensitive(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "CNAME", Name: "www.example.com", Content: "CDN.Example.COM"},
	}
	actual := []Record{
		{Type: TypeCNAME, Name: "www.example.com", Value: "cdn.example.com"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 1)
	assert.True(t, result[0].Match, "comparison should be case-insensitive")
}

func TestCompareRecords_MixedTypes(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
		{Type: "TXT", Name: "example.com", Content: "v=spf1 include:_spf.google.com ~all"},
	}
	actual := []Record{
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
		{Type: TypeTXT, Name: "example.com", Value: "v=spf1 include:_spf.google.com ~all"},
		{Type: TypeNS, Name: "example.com", Value: "ns1.example.com"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 3) // 2 matched + 1 extra NS

	matched := 0
	extra := 0
	for _, r := range result {
		if r.Match {
			matched++
		} else {
			extra++
		}
	}
	assert.Equal(t, 2, matched)
	assert.Equal(t, 1, extra)
}

func TestCompareRecords_SortedOutput(t *testing.T) {
	expected := []dnsprovider.Record{
		{Type: "TXT", Name: "example.com", Content: "hello"},
		{Type: "A", Name: "example.com", Content: "1.2.3.4"},
	}
	actual := []Record{
		{Type: TypeTXT, Name: "example.com", Value: "hello"},
		{Type: TypeA, Name: "example.com", Value: "1.2.3.4"},
	}

	result := CompareRecords(expected, actual)
	require.Len(t, result, 2)
	assert.Equal(t, "A", result[0].Type, "A should come before TXT in sorted output")
	assert.Equal(t, "TXT", result[1].Type)
}

// ── normaliseKey unit tests ──────────────────────────────────────────────────

func TestNormaliseKey(t *testing.T) {
	tests := []struct {
		rtype, value, expected string
	}{
		{"A", "1.2.3.4", "A|1.2.3.4"},
		{"cname", "CDN.Example.COM.", "CNAME|cdn.example.com"},
		{"MX", "mail.example.com", "MX|mail.example.com"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.expected, normaliseKey(tc.rtype, tc.value))
	}
}
