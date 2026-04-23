// Package probe implements the three-tier probe engine (PC.1).
//
// Tier 1 (L1) — infrastructure availability: DNS resolution, TCP 80/443, HTTP status, TLS handshake.
// Tier 2 (L2) — release verification: HTTP keyword, <meta name="release-version">, content hash.
// Tier 3 (L3) — business health: configurable health endpoint with assertion.
package probe

import (
	"context"
	"time"
)

// Status values written to probe_results.status.
const (
	StatusOK      = "ok"
	StatusFail    = "fail"
	StatusTimeout = "timeout"
	StatusError   = "error"
)

// CheckResult is the normalised output of any tier checker.
// It maps directly onto a probe_results row.
type CheckResult struct {
	Status         string      // ok | fail | timeout | error
	HTTPStatus     *int        // HTTP response code (nil if no HTTP response)
	ResponseTimeMS *int        // end-to-end latency in milliseconds
	ResponseSizeB  *int        // body size in bytes (nil if not read)
	TLSHandshakeOK *bool       // nil if no TLS attempt made
	CertExpiresAt  *time.Time  // TLS cert expiry (nil if no TLS)
	ContentHash    *string     // SHA-256 hex of response body (nil if body not read)
	ErrorMessage   *string     // non-nil on StatusFail / StatusTimeout / StatusError
	Detail         interface{} // tier-specific struct, serialised to JSONB
}

// Checker is implemented by L1Checker, L2Checker, L3Checker.
type Checker interface {
	// Tier returns 1, 2, or 3.
	Tier() int16
	// Check runs the probe for the given FQDN and policy parameters.
	Check(ctx context.Context, req CheckRequest) CheckResult
}

// CheckRequest carries all parameters a checker needs.
type CheckRequest struct {
	FQDN string
	// Policy fields — checker only reads what it needs.
	ExpectedStatus  *int    // L2: expected HTTP status (default 200 if nil)
	ExpectedKeyword *string // L2: substring that must appear in response body
	ExpectedMetaTag *string // L2: expected content of <meta name="release-version">
	TimeoutSeconds  int     // timeout for the full check
	// L3 only
	HealthPath string // e.g. "/health" — appended to https://fqdn
}

// strPtr / intPtr / boolPtr are convenience helpers for building *T from T.
func strPtr(s string) *string   { return &s }
func intPtr(i int) *int         { return &i }
func boolPtr(b bool) *bool      { return &b }
func msElapsed(start time.Time) *int {
	ms := int(time.Since(start).Milliseconds())
	return &ms
}
