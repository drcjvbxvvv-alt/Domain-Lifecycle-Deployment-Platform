package checker

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	miekgdns "github.com/miekg/dns"
	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
)

const (
	// TCPProbePort is the default port used for TCP + TLS checks.
	TCPProbePort = 443

	dialTimeout = 5 * time.Second
	tlsTimeout  = 8 * time.Second
	httpTimeout = 8 * time.Second
	dnsTimeout  = 5 * time.Second
)

// Checker executes the 4-layer GFW measurement check for a single FQDN.
// It is safe for concurrent use.
type Checker struct {
	// Resolver is the DNS resolver address in "IP:port" format.
	// Empty string falls back to "8.8.8.8:53".
	Resolver string

	// Bogons is the set of known GFW-injected IPs.  If the zero value is
	// passed, DefaultBogonList() is used.
	Bogons BogonList

	// ProbeVersion is embedded in the HTTP User-Agent header.
	ProbeVersion string

	logger *zap.Logger
}

// New creates a Checker with the given resolver and bogon list.
// Pass an empty string for resolver to use the default (8.8.8.8:53).
// Pass a zero BogonList to use DefaultBogonList.
func New(resolver string, bogons BogonList, probeVersion string, logger *zap.Logger) *Checker {
	if resolver == "" {
		resolver = "8.8.8.8:53"
	}
	if len(bogons.set) == 0 {
		bogons = DefaultBogonList()
	}
	if probeVersion == "" {
		probeVersion = "dev"
	}
	return &Checker{
		Resolver:     resolver,
		Bogons:       bogons,
		ProbeVersion: probeVersion,
		logger:       logger,
	}
}

// FullCheck executes DNS → TCP → TLS → HTTP for the assignment and returns
// the complete Measurement.  The function never returns an error; failures
// at each layer are captured in the corresponding result's Error field.
func (c *Checker) FullCheck(ctx context.Context, a probeprotocol.Assignment, nodeID, nodeRole string) probeprotocol.Measurement {
	start := time.Now()

	m := probeprotocol.Measurement{
		AssignmentID: a.AssignmentID,
		DomainID:     a.DomainID,
		FQDN:         a.FQDN,
		NodeID:       nodeID,
		NodeRole:     nodeRole,
		MeasuredAt:   start,
	}

	// ── Layer 1: DNS ──────────────────────────────────────────────────────
	dnsResult := c.CheckDNS(ctx, a.FQDN)
	m.DNS = &dnsResult

	if dnsResult.Error != "" || len(dnsResult.Answers) == 0 {
		m.TotalMS = time.Since(start).Milliseconds()
		c.logger.Debug("measurement: DNS failed / no answers",
			zap.String("fqdn", a.FQDN),
			zap.String("error", dnsResult.Error),
		)
		return m
	}

	// ── Layers 2 + 3: TCP + TLS per resolved IP ───────────────────────────
	for _, ip := range dnsResult.Answers {
		tcpResult := c.CheckTCP(ctx, ip, TCPProbePort)
		m.TCP = append(m.TCP, tcpResult)

		if tcpResult.Success {
			tlsResult := c.CheckTLS(ctx, ip, a.FQDN)
			m.TLS = append(m.TLS, tlsResult)
		}
	}

	// ── Layer 4: HTTP ─────────────────────────────────────────────────────
	httpResult := c.CheckHTTP(ctx, a.FQDN)
	m.HTTP = &httpResult

	m.TotalMS = time.Since(start).Milliseconds()

	c.logger.Debug("measurement complete",
		zap.String("fqdn", a.FQDN),
		zap.Int64("total_ms", m.TotalMS),
		zap.Bool("dns_bogon", dnsResult.IsBogon),
		zap.Bool("dns_injected", dnsResult.IsInjected),
	)

	return m
}

// CheckDNS resolves fqdn using the configured resolver (raw miekg/dns —
// bypasses the stub resolver so we always see what the GFW returns).
// GFW heuristic flags (IsBogon, IsInjected) are set on the result.
func (c *Checker) CheckDNS(ctx context.Context, fqdn string) probeprotocol.DNSResult {
	result := probeprotocol.DNSResult{
		ResolverIP: c.Resolver,
	}

	start := time.Now()
	client := &miekgdns.Client{Timeout: dnsTimeout}

	msg := new(miekgdns.Msg)
	msg.SetQuestion(miekgdns.Fqdn(fqdn), miekgdns.TypeA)
	msg.RecursionDesired = true

	resp, _, err := client.ExchangeContext(ctx, msg, c.Resolver)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	if resp.Truncated {
		result.Truncated = true
		// Retry over TCP to get the full answer.
		tcpClient := &miekgdns.Client{Net: "tcp", Timeout: dnsTimeout}
		resp, _, err = tcpClient.ExchangeContext(ctx, msg, c.Resolver)
		if err != nil {
			result.Error = "udp_truncated_tcp_retry_failed: " + err.Error()
			return result
		}
	}

	for _, ans := range resp.Answer {
		switch rr := ans.(type) {
		case *miekgdns.A:
			result.Answers = append(result.Answers, rr.A.String())
		case *miekgdns.AAAA:
			result.Answers = append(result.Answers, rr.AAAA.String())
		case *miekgdns.CNAME:
			result.CNAME = append(result.CNAME, rr.Target)
		}
	}

	// ── GFW heuristics ────────────────────────────────────────────────────
	// Bogon check: any resolved IP matches the known GFW injection list.
	result.IsBogon = c.Bogons.AnyBogon(result.Answers)

	// Injection timing: response < 5 ms strongly suggests in-path injection.
	// Only flag when we actually got answers (not an error / NXDOMAIN).
	if len(result.Answers) > 0 {
		result.IsInjected = IsLikelyInjected(result.DurationMS)
	}

	return result
}

// CheckTCP attempts a TCP connection to ip:port and returns the result.
func (c *Checker) CheckTCP(ctx context.Context, ip string, port int) probeprotocol.TCPResult {
	result := probeprotocol.TCPResult{
		IP:   ip,
		Port: port,
	}

	addr := fmt.Sprintf("%s:%d", ip, port)
	start := time.Now()

	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = classifyNetError(err)
		return result
	}
	conn.Close()
	result.Success = true
	return result
}

// CheckTLS performs a TLS handshake to ip:443 with the given SNI.
// On error the Error field is set to a classified label ("connection_reset",
// "timeout", "cert_error") suitable for GFW blocking classification.
func (c *Checker) CheckTLS(ctx context.Context, ip, sni string) probeprotocol.TLSResult {
	result := probeprotocol.TLSResult{
		IP:  ip,
		SNI: sni,
	}

	addr := fmt.Sprintf("%s:%d", ip, TCPProbePort)
	tlsCtx, cancel := context.WithTimeout(ctx, tlsTimeout)
	defer cancel()

	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: false, //nolint:gosec // Cert errors are meaningful here
			MinVersion:         tls.VersionTLS12,
		},
	}

	start := time.Now()
	conn, err := dialer.DialContext(tlsCtx, "tcp", addr)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = classifyTLSError(err)
		return result
	}
	defer conn.Close()

	tlsConn := conn.(*tls.Conn)
	state := tlsConn.ConnectionState()
	result.Success = true

	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		result.CertSubject = cert.Subject.CommonName
		if len(cert.Issuer.Organization) > 0 {
			result.CertIssuer = cert.Issuer.Organization[0]
		}
		result.CertExpiry = cert.NotAfter.Format(time.RFC3339)
	}

	return result
}

// CheckHTTP performs a GET to https://<fqdn>/ and returns the result.
// Redirects are followed up to 3 hops.
func (c *Checker) CheckHTTP(ctx context.Context, fqdn string) probeprotocol.HTTPResult {
	url := "https://" + fqdn + "/"
	result := probeprotocol.HTTPResult{URL: url}

	httpCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout: tlsTimeout,
			// Disable keep-alives so each check starts a fresh connection —
			// important for GFW connection-reset detection.
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "domain-platform-probe/"+c.ProbeVersion)

	start := time.Now()
	resp, err := client.Do(req)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = classifyHTTPError(err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.BodyLength = resp.ContentLength

	// Extract a limited set of response headers useful for detection.
	result.Headers = make(map[string]string)
	for _, key := range []string{"Server", "Content-Type", "Location", "X-Cache", "CF-RAY"} {
		if v := resp.Header.Get(key); v != "" {
			result.Headers[key] = v
		}
	}

	// Extract page title for HTTP-diff detection (PD.3).
	result.Title = extractTitle(resp.Header.Get("Content-Type"), resp.Body)

	return result
}

// ── Error classifiers ─────────────────────────────────────────────────────────

// classifyNetError maps generic network errors to GFW-meaningful labels.
func classifyNetError(err error) string {
	s := err.Error()
	switch {
	case containsAny(s, "connection reset", "connection_reset", "econnreset"):
		return "connection_reset"
	case containsAny(s, "timeout", "deadline exceeded", "i/o timeout"):
		return "timeout"
	case containsAny(s, "refused"):
		return "connection_refused"
	case containsAny(s, "no route", "network unreachable"):
		return "no_route"
	default:
		return s
	}
}

// classifyTLSError maps TLS dial errors to structured labels.
// Labels are used by PD.3 blocking classification.
func classifyTLSError(err error) string {
	s := err.Error()
	switch {
	case containsAny(s, "connection reset", "connection_reset", "econnreset"):
		return "connection_reset"
	case containsAny(s, "timeout", "deadline exceeded", "i/o timeout"):
		return "timeout"
	case containsAny(s, "certificate", " cert", "x509", "unknown authority"):
		return "cert_error"
	case containsAny(s, "handshake failure", "no supported versions", "alert"):
		return "tls_handshake_failure"
	default:
		return s
	}
}

// classifyHTTPError maps HTTP-level errors to structured labels.
func classifyHTTPError(err error) string {
	s := err.Error()
	switch {
	case containsAny(s, "connection reset", "connection_reset", "econnreset", "EOF"):
		return "connection_reset"
	case containsAny(s, "timeout", "deadline exceeded", "i/o timeout"):
		return "timeout"
	case containsAny(s, "tls", "certificate", "x509"):
		return "tls_error"
	case containsAny(s, "too many redirects"):
		return "too_many_redirects"
	default:
		return s
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// containsAny reports whether s contains any of the given substrings
// (case-insensitive).
func containsAny(s string, subs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// extractTitle reads up to 8 KB of the response body looking for an HTML
// <title> tag.  It returns an empty string if none is found or if the
// content type is not HTML.  The caller must close resp.Body.
func extractTitle(contentType string, body interface{ Read([]byte) (int, error) }) string {
	if !strings.Contains(strings.ToLower(contentType), "html") {
		return ""
	}

	buf := make([]byte, 8192)
	n, _ := body.Read(buf)
	if n == 0 {
		return ""
	}
	chunk := string(buf[:n])

	lower := strings.ToLower(chunk)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	// Skip to end of opening tag.
	open := strings.Index(lower[start:], ">")
	if open < 0 {
		return ""
	}
	contentStart := start + open + 1

	end := strings.Index(lower[contentStart:], "</title>")
	if end < 0 {
		return ""
	}

	title := strings.TrimSpace(chunk[contentStart : contentStart+end])
	// Truncate extremely long titles.
	if len(title) > 256 {
		title = title[:256]
	}

	// Collapse internal whitespace.
	return strings.Join(strings.Fields(title), " ")
}

// extractTitleFromBytes wraps extractTitle for byte-slice bodies.
// Used in tests.
func extractTitleFromBytes(contentType string, data []byte) string {
	return extractTitle(contentType, bytes.NewReader(data))
}
