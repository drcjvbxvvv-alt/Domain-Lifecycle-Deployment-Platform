package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	miekgdns "github.com/miekg/dns"
	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
)

const (
	tcpProbePort  = 443
	dialTimeout   = 5 * time.Second
	tlsTimeout    = 8 * time.Second
	httpTimeout   = 8 * time.Second
	dnsTimeout    = 5 * time.Second
)

// measureLoop periodically fetches assignments and runs the 4-layer check for each.
func measureLoop(ctx context.Context, client *http.Client, baseURL, nodeID, nodeRole, dnsResolver string, intervalSecs int, logger *zap.Logger) {
	if intervalSecs <= 0 {
		intervalSecs = 180
	}

	// Run immediately on startup, then on interval.
	runMeasurements(ctx, client, baseURL, nodeID, nodeRole, dnsResolver, logger)

	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runMeasurements(ctx, client, baseURL, nodeID, nodeRole, dnsResolver, logger)
		}
	}
}

func runMeasurements(ctx context.Context, client *http.Client, baseURL, nodeID, nodeRole, dnsResolver string, logger *zap.Logger) {
	assignments, err := fetchAssignments(ctx, client, baseURL, nodeID)
	if err != nil {
		logger.Warn("fetch assignments failed", zap.String("node_id", nodeID), zap.Error(err))
		return
	}

	if len(assignments) == 0 {
		logger.Debug("no assignments for this node", zap.String("node_id", nodeID))
		return
	}

	logger.Info("starting measurement batch",
		zap.String("node_id", nodeID),
		zap.Int("count", len(assignments)),
	)

	results := make([]probeprotocol.Measurement, 0, len(assignments))
	for _, a := range assignments {
		m := runCheck(ctx, a, nodeID, nodeRole, dnsResolver, logger)
		results = append(results, m)
	}

	if err := submitMeasurements(ctx, client, baseURL, nodeID, results, logger); err != nil {
		logger.Warn("submit measurements failed", zap.String("node_id", nodeID), zap.Error(err))
	}
}

// runCheck executes the 4-layer measurement (DNS → TCP → TLS → HTTP) for one assignment.
func runCheck(ctx context.Context, a probeprotocol.Assignment, nodeID, nodeRole, dnsResolver string, logger *zap.Logger) probeprotocol.Measurement {
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
	dnsResult := probeDNS(ctx, a.FQDN, dnsResolver)
	m.DNS = &dnsResult

	// If DNS failed, skip the remaining layers.
	if dnsResult.Error != "" || len(dnsResult.Answers) == 0 {
		m.TotalMS = time.Since(start).Milliseconds()
		logger.Debug("measurement: DNS failed",
			zap.String("fqdn", a.FQDN),
			zap.String("error", dnsResult.Error),
		)
		return m
	}

	// ── Layers 2-4: TCP, TLS, HTTP per resolved IP ────────────────────────
	for _, ip := range dnsResult.Answers {
		// Layer 2: TCP
		tcpResult := probeTCP(ctx, ip, tcpProbePort)
		m.TCP = append(m.TCP, tcpResult)

		// Layer 3: TLS (only attempt if TCP succeeded)
		if tcpResult.Success {
			tlsResult := probeTLS(ctx, ip, a.FQDN)
			m.TLS = append(m.TLS, tlsResult)
		}
	}

	// Layer 4: HTTP (one request to the FQDN, using the first working IP)
	httpResult := probeHTTP(ctx, a.FQDN)
	m.HTTP = &httpResult

	m.TotalMS = time.Since(start).Milliseconds()

	logger.Debug("measurement complete",
		zap.String("fqdn", a.FQDN),
		zap.Int64("total_ms", m.TotalMS),
	)

	return m
}

// probeDNS resolves the FQDN using miekg/dns (raw DNS protocol — avoids stdlib stub resolver).
// dnsResolver is "IP:port", e.g. "8.8.8.8:53". Empty string falls back to "8.8.8.8:53".
func probeDNS(ctx context.Context, fqdn, dnsResolver string) probeprotocol.DNSResult {
	if dnsResolver == "" {
		dnsResolver = "8.8.8.8:53"
	}

	result := probeprotocol.DNSResult{
		ResolverIP: dnsResolver,
	}

	start := time.Now()
	c := new(miekgdns.Client)
	c.Timeout = dnsTimeout

	m := new(miekgdns.Msg)
	m.SetQuestion(miekgdns.Fqdn(fqdn), miekgdns.TypeA)
	m.RecursionDesired = true

	r, _, err := c.ExchangeContext(ctx, m, dnsResolver)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	if r.Truncated {
		result.Truncated = true
		// Retry over TCP
		c.Net = "tcp"
		r, _, err = c.ExchangeContext(ctx, m, dnsResolver)
		if err != nil {
			result.Error = "udp_truncated_tcp_retry_failed: " + err.Error()
			return result
		}
	}

	for _, ans := range r.Answer {
		switch rr := ans.(type) {
		case *miekgdns.A:
			result.Answers = append(result.Answers, rr.A.String())
		case *miekgdns.AAAA:
			result.Answers = append(result.Answers, rr.AAAA.String())
		case *miekgdns.CNAME:
			result.CNAME = append(result.CNAME, rr.Target)
		}
	}

	return result
}

// probeTCP attempts a TCP connect to ip:port and returns the result.
func probeTCP(ctx context.Context, ip string, port int) probeprotocol.TCPResult {
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
		result.Error = err.Error()
		return result
	}
	conn.Close()
	result.Success = true
	return result
}

// probeTLS performs a TLS handshake to ip:443 with the given SNI.
func probeTLS(ctx context.Context, ip, sni string) probeprotocol.TLSResult {
	result := probeprotocol.TLSResult{
		IP:  ip,
		SNI: sni,
	}

	addr := fmt.Sprintf("%s:%d", ip, tcpProbePort)
	start := time.Now()

	tlsCtx, cancel := context.WithTimeout(ctx, tlsTimeout)
	defer cancel()

	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: false, //nolint:gosec // We want to detect cert errors
			MinVersion:         tls.VersionTLS12,
		},
	}

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

// probeHTTP performs an HTTP GET to https://<fqdn>/ and returns the result.
func probeHTTP(ctx context.Context, fqdn string) probeprotocol.HTTPResult {
	url := "https://" + fqdn + "/"
	result := probeprotocol.HTTPResult{URL: url}

	httpCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout: tlsTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet, url, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "domain-platform-probe/"+probeVersion)

	resp, err := httpClient.Do(req)
	result.DurationMS = time.Since(start).Milliseconds()

	if err != nil {
		result.Error = classifyHTTPError(err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.BodyLength = resp.ContentLength
	// Copy response headers (first value per key, limited set)
	result.Headers = map[string]string{}
	for _, key := range []string{"Server", "Content-Type", "Location", "X-Cache"} {
		if v := resp.Header.Get(key); v != "" {
			result.Headers[key] = v
		}
	}

	return result
}

// classifyTLSError maps TLS dial errors to structured labels.
func classifyTLSError(err error) string {
	s := err.Error()
	switch {
	case containsAny(s, "connection reset", "connection_reset"):
		return "connection_reset"
	case containsAny(s, "timeout", "deadline exceeded"):
		return "timeout"
	case containsAny(s, "certificate", "cert"):
		return "cert_error"
	default:
		return s
	}
}

// classifyHTTPError maps HTTP errors to structured labels.
func classifyHTTPError(err error) string {
	s := err.Error()
	switch {
	case containsAny(s, "connection reset"):
		return "connection_reset"
	case containsAny(s, "timeout", "deadline exceeded"):
		return "timeout"
	case containsAny(s, "tls", "certificate"):
		return "tls_error"
	default:
		return s
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// fetchAssignments GETs the domain assignments for this probe node.
func fetchAssignments(ctx context.Context, client *http.Client, baseURL, nodeID string) ([]probeprotocol.Assignment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/probe/v1/assignments?node_id="+nodeID, nil)
	if err != nil {
		return nil, fmt.Errorf("build assignments request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("assignments GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("assignments: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Data probeprotocol.AssignmentsResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode assignments response: %w", err)
	}

	return result.Data.Assignments, nil
}

// submitMeasurements POSTs measurement results to the control plane.
func submitMeasurements(ctx context.Context, client *http.Client, baseURL, nodeID string, measurements []probeprotocol.Measurement, logger *zap.Logger) error {
	payload := probeprotocol.SubmitMeasurementsRequest{
		NodeID:       nodeID,
		Measurements: measurements,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal measurements: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/probe/v1/measurements", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build measurements request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("measurements POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("submit measurements: unexpected status %d", resp.StatusCode)
	}

	logger.Info("measurements submitted",
		zap.String("node_id", nodeID),
		zap.Int("count", len(measurements)),
	)
	return nil
}
