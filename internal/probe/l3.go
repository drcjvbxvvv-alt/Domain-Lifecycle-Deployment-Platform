package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// L3Detail is stored in probe_results.detail for L3 checks.
type L3Detail struct {
	HealthURL       string `json:"health_url"`
	ResponseSnippet string `json:"response_snippet,omitempty"` // first 512 bytes of body
	KeywordFound    bool   `json:"keyword_found,omitempty"`
	Keyword         string `json:"keyword,omitempty"`
}

// L3Checker performs tier-3 business-health probes:
//   HTTP GET to /health (or a configured path) → status 200 + optional keyword.
//
// The health path is taken from CheckRequest.HealthPath.
// If empty, defaults to "/health".
type L3Checker struct{}

func NewL3Checker() *L3Checker { return &L3Checker{} }

func (c *L3Checker) Tier() int16 { return 3 }

func (c *L3Checker) Check(ctx context.Context, req CheckRequest) CheckResult {
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	healthPath := req.HealthPath
	if healthPath == "" {
		healthPath = "/health"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}

	detail := &L3Detail{}
	start := time.Now()

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: false},
			DisableKeepAlives: true,
		},
	}

	// Try HTTPS first, fall back to HTTP.
	var (
		resp    *http.Response
		httpErr error
	)
	for _, scheme := range []string{"https", "http"} {
		url := scheme + "://" + req.FQDN + healthPath
		detail.HealthURL = url
		httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		httpReq.Header.Set("User-Agent", "domain-platform-probe/1.0")
		resp, httpErr = client.Do(httpReq)
		if httpErr == nil {
			break
		}
	}

	elapsed := msElapsed(start)

	if httpErr != nil {
		msg := fmt.Sprintf("http: %v", httpErr)
		return CheckResult{
			Status:         statusFromErr(httpErr),
			ResponseTimeMS: elapsed,
			ErrorMessage:   strPtr(msg),
			Detail:         detail,
		}
	}
	defer resp.Body.Close()

	httpStatus := resp.StatusCode

	// Read up to 512 bytes for snippet + keyword check.
	snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	elapsed = msElapsed(start)
	snippet := string(snip)
	if len(snippet) > 0 {
		detail.ResponseSnippet = snippet
	}

	// Optional keyword assertion.
	if req.ExpectedKeyword != nil && *req.ExpectedKeyword != "" {
		detail.Keyword = *req.ExpectedKeyword
		detail.KeywordFound = strings.Contains(snippet, *req.ExpectedKeyword)
	}

	// ── Determine status ─────────────────────────────────────────────────
	failures := []string{}
	if httpStatus < 200 || httpStatus >= 300 {
		failures = append(failures, fmt.Sprintf("status %d", httpStatus))
	}
	if req.ExpectedKeyword != nil && *req.ExpectedKeyword != "" && !detail.KeywordFound {
		failures = append(failures, fmt.Sprintf("keyword %q not found", *req.ExpectedKeyword))
	}

	status := StatusOK
	var errMsg *string
	if len(failures) > 0 {
		status = StatusFail
		msg := strings.Join(failures, "; ")
		errMsg = &msg
	}

	return CheckResult{
		Status:         status,
		HTTPStatus:     intPtr(httpStatus),
		ResponseTimeMS: elapsed,
		ErrorMessage:   errMsg,
		Detail:         detail,
	}
}
