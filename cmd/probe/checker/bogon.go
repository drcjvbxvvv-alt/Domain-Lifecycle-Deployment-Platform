// Package checker implements the 4-layer GFW measurement check executed on
// each probe and control node.  It is intentionally free of side-effects:
// no file writes, no exec, no credentials.
package checker

// injectionThresholdMS is the maximum round-trip time in milliseconds at
// which a DNS response is considered potentially injected by the GFW.
// Legitimate DNS responses from even nearby resolvers rarely arrive this fast;
// GFW injection typically appears < 2 ms because the reset packet is injected
// in-path before the real resolver responds.
const injectionThresholdMS = 5

// DefaultBogonIPs is the built-in list of IP addresses known to be returned
// by the GFW DNS injection infrastructure.  The list mirrors the seed rows in
// gfw_bogon_ips and MUST be kept in sync.
//
// Sources: OONI measurements, citizenlab/filtering-data, Censorship Canary.
var DefaultBogonIPs = []string{
	"1.2.3.4",
	"37.235.1.174",
	"8.7.198.45",
	"46.82.174.68",
	"78.16.49.15",
	"93.46.8.89",
	"93.46.8.90",
	"243.185.187.39",
	"243.185.187.30",
	"0.0.0.0",
	"127.0.0.1",
}

// BogonList is an immutable set of known GFW-injected IP addresses.
// Construct via NewBogonList; the zero value is an empty (non-nil) set.
type BogonList struct {
	set map[string]struct{}
}

// NewBogonList builds a BogonList from the provided IP address strings.
// Passing nil or an empty slice produces a non-nil list with zero entries.
func NewBogonList(ips []string) BogonList {
	m := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if ip != "" {
			m[ip] = struct{}{}
		}
	}
	return BogonList{set: m}
}

// DefaultBogonList returns a BogonList seeded with DefaultBogonIPs.
func DefaultBogonList() BogonList {
	return NewBogonList(DefaultBogonIPs)
}

// Contains reports whether ip is a known bogon.
func (b BogonList) Contains(ip string) bool {
	_, ok := b.set[ip]
	return ok
}

// AnyBogon returns true if at least one IP in ips is in the bogon list.
func (b BogonList) AnyBogon(ips []string) bool {
	for _, ip := range ips {
		if b.Contains(ip) {
			return true
		}
	}
	return false
}

// IsLikelyInjected returns true when durationMS is below the injection
// threshold, indicating that the DNS response may have been synthesised
// in-path by the GFW rather than returned by a real resolver.
func IsLikelyInjected(durationMS int64) bool {
	return durationMS >= 0 && durationMS < injectionThresholdMS
}
