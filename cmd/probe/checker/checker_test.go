package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── BogonList ─────────────────────────────────────────────────────────────────

func TestBogonList_Contains(t *testing.T) {
	bl := NewBogonList([]string{"1.2.3.4", "0.0.0.0"})

	assert.True(t, bl.Contains("1.2.3.4"), "known bogon should be detected")
	assert.True(t, bl.Contains("0.0.0.0"), "null route should be detected")
	assert.False(t, bl.Contains("8.8.8.8"), "legitimate IP should not be flagged")
	assert.False(t, bl.Contains("104.21.0.1"), "Cloudflare IP should not be flagged")
	assert.False(t, bl.Contains(""), "empty string should not match")
}

func TestBogonList_AnyBogon(t *testing.T) {
	bl := DefaultBogonList()

	tests := []struct {
		name string
		ips  []string
		want bool
	}{
		{"only bogon", []string{"1.2.3.4"}, true},
		{"bogon mixed with real", []string{"104.21.1.1", "37.235.1.174"}, true},
		{"all real IPs", []string{"8.8.8.8", "1.1.1.1", "104.21.0.1"}, false},
		{"empty list", []string{}, false},
		{"nil list", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bl.AnyBogon(tt.ips))
		})
	}
}

func TestDefaultBogonList_ContainsKnownIPs(t *testing.T) {
	bl := DefaultBogonList()

	knownBogons := DefaultBogonIPs
	require.NotEmpty(t, knownBogons)

	for _, ip := range knownBogons {
		assert.True(t, bl.Contains(ip), "default list should contain %q", ip)
	}
}

func TestNewBogonList_EmptyInput(t *testing.T) {
	bl := NewBogonList(nil)
	assert.False(t, bl.Contains("1.2.3.4"))
	assert.False(t, bl.AnyBogon([]string{"1.2.3.4"}))
}

func TestNewBogonList_SkipsEmptyStrings(t *testing.T) {
	bl := NewBogonList([]string{"", "1.2.3.4", ""})
	assert.True(t, bl.Contains("1.2.3.4"))
	assert.False(t, bl.Contains(""))
}

// ── IsLikelyInjected ──────────────────────────────────────────────────────────

func TestIsLikelyInjected(t *testing.T) {
	tests := []struct {
		name       string
		durationMS int64
		want       bool
	}{
		{"0ms — definitely injected", 0, true},
		{"1ms — injected", 1, true},
		{"4ms — just under threshold", 4, true},
		{"5ms — at threshold (not injected)", 5, false},
		{"10ms — fast but not injected", 10, false},
		{"100ms — normal DNS", 100, false},
		{"negative — never injected", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLikelyInjected(tt.durationMS))
		})
	}
}

// ── Error classifiers ─────────────────────────────────────────────────────────

func TestClassifyNetError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"connection reset", "read: connection reset by peer", "connection_reset"},
		{"econnreset", "econnreset error occurred", "connection_reset"},
		{"timeout", "dial timeout exceeded", "timeout"},
		{"deadline exceeded", "context deadline exceeded", "timeout"},
		{"i/o timeout", "i/o timeout on read", "timeout"},
		{"connection refused", "connection refused", "connection_refused"},
		{"no route", "no route to host", "no_route"},
		{"network unreachable", "network unreachable", "no_route"},
		{"unknown", "some exotic error", "some exotic error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyNetError(errString(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyTLSError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"connection reset", "read: connection reset by peer", "connection_reset"},
		{"timeout", "dial timeout exceeded", "timeout"},
		{"deadline", "context deadline exceeded", "timeout"},
		{"x509", "x509 certificate signed by unknown authority", "cert_error"},
		{"cert keyword", "TLS cert validation failed", "cert_error"},
		{"unknown authority", "unknown certificate authority", "cert_error"},
		{"handshake failure", "tls: handshake failure", "tls_handshake_failure"},
		{"alert", "remote error: tls alert 40", "tls_handshake_failure"},
		{"unknown", "some other tls error", "some other tls error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTLSError(errString(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"connection reset", "connection reset by peer", "connection_reset"},
		{"EOF", "unexpected EOF", "connection_reset"},
		{"timeout", "connection timeout exceeded", "timeout"},
		{"deadline", "context deadline exceeded", "timeout"},
		{"tls error", "tls handshake failed", "tls_error"},
		{"certificate", "certificate verify failed", "tls_error"},
		{"too many redirects", "stopped after too many redirects", "too_many_redirects"},
		{"unknown", "proxy error: 502", "proxy error: 502"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHTTPError(errString(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ── containsAny ───────────────────────────────────────────────────────────────

func TestContainsAny(t *testing.T) {
	assert.True(t, containsAny("Connection reset by peer", "connection reset"))
	assert.True(t, containsAny("TIMEOUT exceeded", "timeout"))
	assert.True(t, containsAny("x509 error", "certificate", "x509"))
	assert.False(t, containsAny("normal response", "timeout", "reset"))
	assert.False(t, containsAny("", "timeout"))
	// Case-insensitive
	assert.True(t, containsAny("ECONNRESET", "econnreset"))
}

// ── extractTitle ──────────────────────────────────────────────────────────────

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			"simple title",
			"text/html; charset=utf-8",
			"<html><head><title>Hello World</title></head></html>",
			"Hello World",
		},
		{
			"title with whitespace",
			"text/html",
			"<html><head><title>  Hello   World  </title></head></html>",
			"Hello World",
		},
		{
			"uppercase TITLE tag",
			"text/html",
			"<HTML><HEAD><TITLE>Blocked Page</TITLE></HEAD></HTML>",
			"Blocked Page",
		},
		{
			"no title tag",
			"text/html",
			"<html><head></head><body>No title</body></html>",
			"",
		},
		{
			"non-HTML content type",
			"application/json",
			`{"title": "should be ignored"}`,
			"",
		},
		{
			"empty body",
			"text/html",
			"",
			"",
		},
		{
			"truncated — no closing tag",
			"text/html",
			"<html><head><title>Incomplete",
			"",
		},
		{
			"GFW block page pattern",
			"text/html; charset=GB2312",
			"<html><head><title>访问受限</title></head></html>",
			"访问受限",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleFromBytes(tt.contentType, []byte(tt.body))
			assert.Equal(t, tt.want, got)
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// errString satisfies the error interface for classifier tests.
type errString string

func (e errString) Error() string { return string(e) }
