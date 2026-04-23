package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// dnsHost strips any port suffix so LookupHost receives a bare hostname / IP.
func dnsHost(fqdn string) string {
	host, _, err := net.SplitHostPort(fqdn)
	if err != nil {
		return fqdn // no port — use as-is
	}
	return host
}

// L1Detail is the tier-specific payload stored in probe_results.detail for L1 checks.
type L1Detail struct {
	DNSResolved bool   `json:"dns_resolved"`
	DNSIPs      []string `json:"dns_ips,omitempty"`
	TCPPort80   bool   `json:"tcp_port_80"`
	TCPPort443  bool   `json:"tcp_port_443"`
	TLSOK       bool   `json:"tls_ok"`
	HTTPStatus  int    `json:"http_status,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

// L1Checker performs tier-1 infrastructure probes:
//   DNS resolution → TCP 80/443 → HTTP GET → TLS handshake
type L1Checker struct{}

func NewL1Checker() *L1Checker { return &L1Checker{} }

func (c *L1Checker) Tier() int16 { return 1 }

func (c *L1Checker) Check(ctx context.Context, req CheckRequest) CheckResult {
	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	detail := &L1Detail{}
	start := time.Now()

	// ── 1. DNS resolution ────────────────────────────────────────────────
	// Strip port if present — LookupHost only accepts a bare hostname.
	bareHost := dnsHost(req.FQDN)
	resolver := &net.Resolver{}
	addrs, dnsErr := resolver.LookupHost(ctx, bareHost)
	if dnsErr != nil {
		msg := fmt.Sprintf("dns: %v", dnsErr)
		return CheckResult{
			Status:         StatusFail,
			ResponseTimeMS: msElapsed(start),
			ErrorMessage:   strPtr(msg),
			Detail:         detail,
		}
	}
	detail.DNSResolved = true
	detail.DNSIPs = addrs

	// ── 2. TCP port probes (best-effort; use bare host + standard ports) ─
	tcpDialer := &net.Dialer{Timeout: 3 * time.Second}
	if conn, err := tcpDialer.DialContext(ctx, "tcp", bareHost+":80"); err == nil {
		conn.Close()
		detail.TCPPort80 = true
	}
	if conn, err := tcpDialer.DialContext(ctx, "tcp", bareHost+":443"); err == nil {
		conn.Close()
		detail.TCPPort443 = true
	}

	// ── 3. HTTP GET (follows redirects, captures status + TLS) ───────────
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: false},
			DisableKeepAlives: true,
		},
	}

	// Try HTTPS first, fall back to HTTP.
	var (
		resp    *http.Response
		httpErr error
		scheme  = "https"
	)
	for _, s := range []string{"https", "http"} {
		url := s + "://" + req.FQDN + "/"
		httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		httpReq.Header.Set("User-Agent", "domain-platform-probe/1.0")
		resp, httpErr = client.Do(httpReq)
		if httpErr == nil {
			scheme = s
			break
		}
	}

	elapsed := msElapsed(start)

	if httpErr != nil {
		msg := fmt.Sprintf("http: %v", httpErr)
		return CheckResult{
			Status:         statusFromErr(httpErr),
			ResponseTimeMS: elapsed,
			Detail:         detail,
			ErrorMessage:   strPtr(msg),
		}
	}
	defer resp.Body.Close()

	httpStatus := resp.StatusCode
	detail.HTTPStatus = httpStatus

	if resp.Request != nil && resp.Request.URL != nil {
		u := resp.Request.URL.String()
		if u != scheme+"://"+req.FQDN+"/" {
			detail.RedirectURL = u
		}
	}

	// ── 4. TLS state ─────────────────────────────────────────────────────
	var certExpiresAt *time.Time
	if resp.TLS != nil {
		detail.TLSOK = true
		// Earliest expiry among all certs in the chain.
		for _, cert := range resp.TLS.PeerCertificates {
			if certExpiresAt == nil || cert.NotAfter.Before(*certExpiresAt) {
				t := cert.NotAfter
				certExpiresAt = &t
			}
		}
	}

	probeStatus := StatusOK
	if httpStatus >= 500 {
		probeStatus = StatusFail
	}

	return CheckResult{
		Status:         probeStatus,
		HTTPStatus:     intPtr(httpStatus),
		ResponseTimeMS: elapsed,
		TLSHandshakeOK: boolPtr(detail.TLSOK),
		CertExpiresAt:  certExpiresAt,
		Detail:         detail,
	}
}

// statusFromErr maps common HTTP error types to probe status strings.
func statusFromErr(err error) string {
	if err == nil {
		return StatusOK
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return StatusTimeout
	}
	return StatusFail
}
