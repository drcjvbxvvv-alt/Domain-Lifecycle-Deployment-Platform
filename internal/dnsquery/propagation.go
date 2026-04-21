package dnsquery

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"go.uber.org/zap"
)

// ── Well-known public resolvers ──────────────────────────────────────────────

// PublicResolver describes a well-known DNS resolver.
type PublicResolver struct {
	Address string `json:"address"` // "8.8.8.8:53"
	Label   string `json:"label"`   // "Google"
}

// DefaultResolvers is the standard set of public resolvers to check.
var DefaultResolvers = []PublicResolver{
	{Address: "8.8.8.8:53", Label: "Google"},
	{Address: "1.1.1.1:53", Label: "Cloudflare"},
	{Address: "9.9.9.9:53", Label: "Quad9"},
	{Address: "208.67.222.222:53", Label: "OpenDNS"},
}

// ── Result types ─────────────────────────────────────────────────────────────

// ResolverResult holds the DNS answer from one specific resolver.
type ResolverResult struct {
	Address      string   `json:"address"`
	Label        string   `json:"label"`
	Records      []Record `json:"records"`
	Authoritative bool    `json:"authoritative"` // true if this is an authoritative NS
	ElapsedMs    int64    `json:"elapsed_ms"`
	Error        string   `json:"error,omitempty"`
}

// PropagationResult is the aggregated result of a propagation check.
type PropagationResult struct {
	FQDN       string           `json:"fqdn"`
	QueryTypes []string         `json:"query_types"`  // e.g. ["A","AAAA"]
	Resolvers  []ResolverResult `json:"resolvers"`
	Consistent bool             `json:"consistent"`   // all resolvers agree
	QueriedAt  string           `json:"queried_at"`
	TotalMs    int64            `json:"total_ms"`
}

// ── Propagation check ────────────────────────────────────────────────────────

// CheckPropagation queries the given FQDN against multiple public resolvers
// and the domain's authoritative nameservers, then compares the results.
//
// queryTypes specifies which DNS record types to check (e.g. ["A","AAAA"]).
// If empty, defaults to A + AAAA.
func (s *Service) CheckPropagation(ctx context.Context, fqdn string, queryTypes []string) *PropagationResult {
	fqdn = strings.TrimSuffix(strings.TrimSpace(fqdn), ".")
	if fqdn == "" {
		return &PropagationResult{FQDN: fqdn, QueriedAt: now(), Consistent: false}
	}

	if len(queryTypes) == 0 {
		queryTypes = []string{"A", "AAAA"}
	}

	start := time.Now()
	result := &PropagationResult{
		FQDN:       fqdn,
		QueryTypes: queryTypes,
		QueriedAt:  now(),
	}

	// Build the resolver list: public + authoritative
	resolvers := make([]resolverEntry, len(DefaultResolvers))
	for i, r := range DefaultResolvers {
		resolvers[i] = resolverEntry{address: r.Address, label: r.Label}
	}

	// Discover authoritative NS
	authNSes := s.discoverAuthNS(ctx, fqdn)
	for _, ns := range authNSes {
		resolvers = append(resolvers, resolverEntry{
			address:       ns,
			label:         fmt.Sprintf("權威 NS: %s", strings.TrimSuffix(ns, ":53")),
			authoritative: true,
		})
	}

	// Query all resolvers concurrently
	var wg sync.WaitGroup
	results := make([]ResolverResult, len(resolvers))

	for i, entry := range resolvers {
		wg.Add(1)
		go func(idx int, e resolverEntry) {
			defer wg.Done()
			results[idx] = s.queryResolver(ctx, fqdn, e, queryTypes)
		}(i, entry)
	}
	wg.Wait()

	result.Resolvers = results
	result.TotalMs = time.Since(start).Milliseconds()
	result.Consistent = checkConsistency(results)

	return result
}

// resolverEntry is internal; bundles address + label + authoritative flag.
type resolverEntry struct {
	address       string
	label         string
	authoritative bool
}

// queryResolver queries one resolver for the specified record types.
func (s *Service) queryResolver(ctx context.Context, fqdn string, entry resolverEntry, queryTypes []string) ResolverResult {
	rr := ResolverResult{
		Address:       entry.address,
		Label:         entry.label,
		Authoritative: entry.authoritative,
	}

	c := new(dns.Client)
	c.Timeout = 5 * time.Second
	qname := dns.Fqdn(fqdn)

	start := time.Now()

	for _, qt := range queryTypes {
		qtype, ok := dns.StringToType[qt]
		if !ok {
			continue
		}

		msg := new(dns.Msg)
		msg.SetQuestion(qname, qtype)
		msg.RecursionDesired = true

		resp, _, err := c.ExchangeContext(ctx, msg, entry.address)
		if err != nil {
			rr.Error = err.Error()
			continue
		}
		if resp == nil {
			continue
		}
		// TCP fallback on truncation
		if resp.Truncated {
			tcpC := new(dns.Client)
			tcpC.Net = "tcp"
			tcpC.Timeout = 5 * time.Second
			if tcpResp, _, tcpErr := tcpC.ExchangeContext(ctx, msg, entry.address); tcpErr == nil && tcpResp != nil {
				resp = tcpResp
			}
		}

		for _, answer := range resp.Answer {
			rec := parseRR(answer)
			if rec != nil {
				rr.Records = append(rr.Records, *rec)
			}
		}
	}

	rr.Records = dedup(rr.Records)
	sort.Slice(rr.Records, func(i, j int) bool {
		oi, oj := typeOrder[rr.Records[i].Type], typeOrder[rr.Records[j].Type]
		if oi != oj {
			return oi < oj
		}
		return rr.Records[i].Value < rr.Records[j].Value
	})

	rr.ElapsedMs = time.Since(start).Milliseconds()
	return rr
}

// discoverAuthNS finds the authoritative nameservers for a domain
// by querying the NS records, then resolving those NS hostnames to IP:port.
func (s *Service) discoverAuthNS(ctx context.Context, fqdn string) []string {
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(fqdn), dns.TypeNS)
	msg.RecursionDesired = true

	resp, _, err := c.ExchangeContext(ctx, msg, s.nameserver)
	if err != nil || resp == nil {
		s.logger.Debug("failed to discover authoritative NS", zap.String("fqdn", fqdn), zap.Error(err))
		return nil
	}

	var nsHosts []string
	for _, rr := range resp.Answer {
		if ns, ok := rr.(*dns.NS); ok {
			nsHosts = append(nsHosts, strings.TrimSuffix(ns.Ns, "."))
		}
	}

	// Resolve each NS hostname to an IP
	var result []string
	for _, host := range nsHosts {
		// Limit to first 2 authoritative NS to keep latency reasonable
		if len(result) >= 2 {
			break
		}
		ips, lookupErr := net.LookupHost(host)
		if lookupErr != nil || len(ips) == 0 {
			s.logger.Debug("cannot resolve NS host", zap.String("ns", host), zap.Error(lookupErr))
			continue
		}
		result = append(result, net.JoinHostPort(ips[0], "53"))
	}

	return result
}

// checkConsistency returns true if all resolvers (that did not error)
// returned the same set of A/AAAA record values. Differences in TTL
// are ignored — only the value matters for propagation.
func checkConsistency(results []ResolverResult) bool {
	var fingerprints []string

	for _, rr := range results {
		if rr.Error != "" {
			continue // skip errored resolvers
		}
		fp := fingerprint(rr.Records)
		fingerprints = append(fingerprints, fp)
	}

	if len(fingerprints) <= 1 {
		return true // 0 or 1 non-error result → trivially consistent
	}

	first := fingerprints[0]
	for _, fp := range fingerprints[1:] {
		if fp != first {
			return false
		}
	}
	return true
}

// fingerprint produces a canonical string from records (type+value, sorted).
// TTL is intentionally excluded — different resolvers cache with different TTLs.
func fingerprint(records []Record) string {
	pairs := make([]string, len(records))
	for i, r := range records {
		pairs[i] = string(r.Type) + "=" + r.Value
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "|")
}
