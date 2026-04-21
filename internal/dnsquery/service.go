// Package dnsquery provides live DNS record lookups using the system resolver.
// It queries public DNS for A, AAAA, CNAME, MX, TXT, NS records and returns
// a unified result. This is a read-only, credential-free approach — suitable
// for "what does this domain resolve to right now?" queries.
package dnsquery

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

// RecordType enumerates the DNS record types we query.
type RecordType string

const (
	TypeA     RecordType = "A"
	TypeAAAA  RecordType = "AAAA"
	TypeCNAME RecordType = "CNAME"
	TypeMX    RecordType = "MX"
	TypeTXT   RecordType = "TXT"
	TypeNS    RecordType = "NS"
)

// Record represents a single DNS record returned by a lookup.
type Record struct {
	Type     RecordType `json:"type"`
	Name     string     `json:"name"`
	Value    string     `json:"value"`
	Priority int        `json:"priority,omitempty"` // MX only
	TTL      int        `json:"ttl,omitempty"`      // not available from net pkg, kept for future
}

// LookupResult is the full DNS lookup response for one FQDN.
type LookupResult struct {
	FQDN      string   `json:"fqdn"`
	Records   []Record `json:"records"`
	QueriedAt string   `json:"queried_at"` // ISO 8601
	Error     string   `json:"error,omitempty"`
}

// Service performs DNS lookups.
type Service struct {
	resolver *net.Resolver
	logger   *zap.Logger
}

// NewService returns a DNS query service using the given resolver.
// Pass nil to use the default system resolver.
func NewService(logger *zap.Logger) *Service {
	return &Service{
		resolver: net.DefaultResolver,
		logger:   logger,
	}
}

// Lookup queries all supported record types for the given FQDN and returns
// the aggregated result. Individual record type failures are logged but do
// not cause the overall lookup to fail — the result simply omits that type.
func (s *Service) Lookup(ctx context.Context, fqdn string) *LookupResult {
	fqdn = strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	if fqdn == "" {
		return &LookupResult{FQDN: fqdn, Error: "empty FQDN", QueriedAt: now()}
	}

	// Use a short timeout per query type so the whole lookup stays snappy.
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := &LookupResult{
		FQDN:      fqdn,
		QueriedAt: now(),
	}

	// ── A + AAAA ──────────────────────────────────────────────────────────
	ips, err := s.resolver.LookupIPAddr(qctx, fqdn)
	if err != nil {
		s.logQueryErr(fqdn, "A/AAAA", err)
	}
	for _, ip := range ips {
		rtype := TypeA
		if ip.IP.To4() == nil {
			rtype = TypeAAAA
		}
		result.Records = append(result.Records, Record{
			Type:  rtype,
			Name:  fqdn,
			Value: ip.IP.String(),
		})
	}

	// ── CNAME ─────────────────────────────────────────────────────────────
	cname, err := s.resolver.LookupCNAME(qctx, fqdn)
	if err == nil && cname != "" {
		canonical := strings.TrimSuffix(cname, ".")
		// Only include if it differs from the queried name (not self-referencing)
		if !strings.EqualFold(canonical, fqdn) {
			result.Records = append(result.Records, Record{
				Type:  TypeCNAME,
				Name:  fqdn,
				Value: canonical,
			})
		}
	}

	// ── MX ────────────────────────────────────────────────────────────────
	mxs, err := s.resolver.LookupMX(qctx, fqdn)
	if err != nil {
		s.logQueryErr(fqdn, "MX", err)
	}
	for _, mx := range mxs {
		result.Records = append(result.Records, Record{
			Type:     TypeMX,
			Name:     fqdn,
			Value:    strings.TrimSuffix(mx.Host, "."),
			Priority: int(mx.Pref),
		})
	}

	// ── TXT ───────────────────────────────────────────────────────────────
	txts, err := s.resolver.LookupTXT(qctx, fqdn)
	if err != nil {
		s.logQueryErr(fqdn, "TXT", err)
	}
	for _, txt := range txts {
		result.Records = append(result.Records, Record{
			Type:  TypeTXT,
			Name:  fqdn,
			Value: txt,
		})
	}

	// ── NS ────────────────────────────────────────────────────────────────
	nss, err := s.resolver.LookupNS(qctx, fqdn)
	if err != nil {
		s.logQueryErr(fqdn, "NS", err)
	}
	for _, ns := range nss {
		result.Records = append(result.Records, Record{
			Type:  TypeNS,
			Name:  fqdn,
			Value: strings.TrimSuffix(ns.Host, "."),
		})
	}

	// Sort: type order (A, AAAA, CNAME, MX, NS, TXT) then value
	typeOrder := map[RecordType]int{TypeA: 0, TypeAAAA: 1, TypeCNAME: 2, TypeMX: 3, TypeNS: 4, TypeTXT: 5}
	sort.Slice(result.Records, func(i, j int) bool {
		if result.Records[i].Type != result.Records[j].Type {
			return typeOrder[result.Records[i].Type] < typeOrder[result.Records[j].Type]
		}
		return result.Records[i].Value < result.Records[j].Value
	})

	return result
}

func (s *Service) logQueryErr(fqdn string, qtype string, err error) {
	// DNS "no such host" is normal for missing record types — debug only
	if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
		s.logger.Debug("dns record not found",
			zap.String("fqdn", fqdn),
			zap.String("type", qtype),
		)
		return
	}
	s.logger.Warn("dns lookup error",
		zap.String("fqdn", fqdn),
		zap.String("type", qtype),
		zap.Error(err),
	)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// LookupMultiple queries DNS for multiple FQDNs concurrently (up to 20).
// Useful for batch views like the domain list table.
func (s *Service) LookupMultiple(ctx context.Context, fqdns []string) []LookupResult {
	const maxConcurrency = 20
	if len(fqdns) > maxConcurrency {
		fqdns = fqdns[:maxConcurrency]
	}

	results := make([]LookupResult, len(fqdns))
	done := make(chan struct{}, len(fqdns))

	for i, fqdn := range fqdns {
		go func(idx int, name string) {
			r := s.Lookup(ctx, name)
			results[idx] = *r
			done <- struct{}{}
		}(i, fqdn)
	}

	for range fqdns {
		<-done
	}

	return results
}
