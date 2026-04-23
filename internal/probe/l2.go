package probe

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// maxBodyBytes is the maximum response body read for L2 analysis (1 MB).
const maxBodyBytes = 1 << 20

// metaTagRe finds any <meta ...> tag; metaNameRe checks it has name="release-version";
// metaContentRe extracts the content attribute value.
// Three-regex approach handles attributes in any order.
var (
	metaTagRe     = regexp.MustCompile(`(?i)<meta\b[^>]*>`)
	metaNameRe    = regexp.MustCompile(`(?i)\bname=["']release-version["']`)
	metaContentRe = regexp.MustCompile(`(?i)\bcontent=["']([^"']+)["']`)
)

// L2Detail is stored in probe_results.detail for L2 checks.
type L2Detail struct {
	KeywordFound        bool   `json:"keyword_found,omitempty"`
	Keyword             string `json:"keyword,omitempty"`
	MetaVersionDetected string `json:"meta_version_detected,omitempty"`
	MetaVersionExpected string `json:"meta_version_expected,omitempty"`
	MetaVersionMatch    bool   `json:"meta_version_match,omitempty"`
	ContentHash         string `json:"content_hash,omitempty"`
}

// L2Checker performs tier-2 release-verification probes:
//   HTTP status check → keyword search → meta release-version → content hash
type L2Checker struct{}

func NewL2Checker() *L2Checker { return &L2Checker{} }

func (c *L2Checker) Tier() int16 { return 2 }

func (c *L2Checker) Check(ctx context.Context, req CheckRequest) CheckResult {
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	detail := &L2Detail{}
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
		url := scheme + "://" + req.FQDN + "/"
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

	// Read body up to maxBodyBytes.
	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	responseSize := len(bodyBytes)
	elapsed = msElapsed(start)

	if readErr != nil {
		msg := fmt.Sprintf("read body: %v", readErr)
		return CheckResult{
			Status:         StatusError,
			HTTPStatus:     intPtr(httpStatus),
			ResponseTimeMS: elapsed,
			ErrorMessage:   strPtr(msg),
			Detail:         detail,
		}
	}

	body := string(bodyBytes)

	// ── SHA-256 content hash ────────────────────────────────────────────
	hash := sha256.Sum256(bodyBytes)
	hashHex := fmt.Sprintf("%x", hash)
	detail.ContentHash = hashHex

	// ── Keyword check ───────────────────────────────────────────────────
	if req.ExpectedKeyword != nil && *req.ExpectedKeyword != "" {
		detail.Keyword = *req.ExpectedKeyword
		detail.KeywordFound = strings.Contains(body, *req.ExpectedKeyword)
	}

	// ── Meta release-version check ──────────────────────────────────────
	if req.ExpectedMetaTag != nil && *req.ExpectedMetaTag != "" {
		detail.MetaVersionExpected = *req.ExpectedMetaTag
		// Walk all <meta> tags; find the one with name="release-version".
		for _, tag := range metaTagRe.FindAllString(body, -1) {
			if metaNameRe.MatchString(tag) {
				if m := metaContentRe.FindStringSubmatch(tag); len(m) > 1 {
					detail.MetaVersionDetected = m[1]
					detail.MetaVersionMatch = m[1] == *req.ExpectedMetaTag
				}
				break
			}
		}
	}

	// ── Determine overall status ────────────────────────────────────────
	expectedStatus := 200
	if req.ExpectedStatus != nil {
		expectedStatus = *req.ExpectedStatus
	}

	failures := []string{}
	if httpStatus != expectedStatus {
		failures = append(failures, fmt.Sprintf("status %d != %d", httpStatus, expectedStatus))
	}
	if req.ExpectedKeyword != nil && *req.ExpectedKeyword != "" && !detail.KeywordFound {
		failures = append(failures, fmt.Sprintf("keyword %q not found", *req.ExpectedKeyword))
	}
	if req.ExpectedMetaTag != nil && *req.ExpectedMetaTag != "" && !detail.MetaVersionMatch {
		failures = append(failures, fmt.Sprintf("meta version %q != %q", detail.MetaVersionDetected, *req.ExpectedMetaTag))
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
		ResponseSizeB:  intPtr(responseSize),
		ContentHash:    strPtr(hashHex),
		ErrorMessage:   errMsg,
		Detail:         detail,
	}
}
