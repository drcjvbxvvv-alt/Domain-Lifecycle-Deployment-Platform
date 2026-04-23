// Package probeprotocol defines the wire types shared between the probe node
// binary (cmd/probe) and the control-plane server (cmd/server /probe/v1/*).
//
// Design mirrors pkg/agentprotocol but for the GFW detection vertical (Phase D).
// Probe nodes are read-only measurement vantage points; they NEVER receive
// write tasks and NEVER hold provider credentials.
package probeprotocol

import "time"

// ── Node roles + statuses ─────────────────────────────────────────────────────

const (
	RoleProbe   = "probe"   // CN vantage point (inside GFW)
	RoleControl = "control" // uncensored vantage point (HK, JP, etc.)
)

const (
	StatusRegistered = "registered"
	StatusOnline     = "online"
	StatusOffline    = "offline"
	StatusError      = "error"
)

// ── Registration ──────────────────────────────────────────────────────────────

// RegisterRequest is sent by the probe node on first contact with the control
// plane (POST /probe/v1/register).
type RegisterRequest struct {
	NodeID       string `json:"node_id"`      // operator-assigned, e.g. "cn-beijing-01"
	Region       string `json:"region"`       // "cn-north", "cn-east", "hk", "jp"
	Role         string `json:"role"`         // "probe" | "control"
	ProbeVersion string `json:"probe_version"`
	IPAddress    string `json:"ip_address,omitempty"`
}

// RegisterResponse is returned by the control plane after successful registration.
type RegisterResponse struct {
	NodeID        string `json:"node_id"`
	Status        string `json:"status"`
	HeartbeatSecs int    `json:"heartbeat_secs"`
	CheckInterval int    `json:"check_interval"` // default measurement interval (seconds)
}

// ── Heartbeat ─────────────────────────────────────────────────────────────────

// HeartbeatRequest is sent periodically by the probe node
// (POST /probe/v1/heartbeat).
type HeartbeatRequest struct {
	NodeID       string  `json:"node_id"`
	ProbeVersion string  `json:"probe_version"`
	LoadAvg1     float64 `json:"load_avg_1"`
	DiskFreePct  float64 `json:"disk_free_pct"`
	ChecksToday  int     `json:"checks_today"`
	LastError    string  `json:"last_error,omitempty"`
}

// HeartbeatResponse is returned by the control plane.
type HeartbeatResponse struct {
	Ack           bool   `json:"ack"`
	HasNewDomains bool   `json:"has_new_domains"` // hint: pull assignments again
}

// ── Assignments ───────────────────────────────────────────────────────────────

// Assignment describes one domain the probe node must check.
type Assignment struct {
	AssignmentID  int64  `json:"assignment_id"`
	DomainID      int64  `json:"domain_id"`
	FQDN          string `json:"fqdn"`
	CheckInterval int    `json:"check_interval"` // seconds
}

// AssignmentsResponse is returned by GET /probe/v1/assignments.
type AssignmentsResponse struct {
	NodeID      string       `json:"node_id"`
	Assignments []Assignment `json:"assignments"`
}

// ── Measurement results ───────────────────────────────────────────────────────

// DNSResult is the result of a DNS lookup at one vantage point.
type DNSResult struct {
	ResolverIP string   `json:"resolver_ip"`
	Answers    []string `json:"answers"`    // resolved IPs (A/AAAA)
	CNAME      []string `json:"cname"`      // CNAME chain (may be empty)
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	Truncated  bool     `json:"truncated,omitempty"` // UDP truncated, retried via TCP
}

// TCPResult is the result of a TCP connect to one IP:port.
type TCPResult struct {
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// TLSResult is the result of a TLS handshake to one IP with a given SNI.
type TLSResult struct {
	IP          string `json:"ip"`
	SNI         string `json:"sni"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"` // "connection_reset", "timeout", "cert_error"
	DurationMS  int64  `json:"duration_ms"`
	CertSubject string `json:"cert_subject,omitempty"`
	CertIssuer  string `json:"cert_issuer,omitempty"`
	CertExpiry  string `json:"cert_expiry,omitempty"` // RFC3339
}

// HTTPResult is the result of an HTTP GET request.
type HTTPResult struct {
	URL        string            `json:"url"`
	StatusCode int               `json:"status_code"`
	BodyLength int64             `json:"body_length"`
	Title      string            `json:"title,omitempty"` // extracted <title>
	Headers    map[string]string `json:"headers,omitempty"`
	Error      string            `json:"error,omitempty"` // "connection_reset", "timeout", "tls_error"
	DurationMS int64             `json:"duration_ms"`
}

// Measurement is the full 4-layer result for one (FQDN, node) pair.
// The probe node submits one Measurement per assigned domain per check cycle.
type Measurement struct {
	AssignmentID int64       `json:"assignment_id"`
	DomainID     int64       `json:"domain_id"`
	FQDN         string      `json:"fqdn"`
	NodeID       string      `json:"node_id"`
	NodeRole     string      `json:"node_role"` // "probe" | "control"
	DNS          *DNSResult  `json:"dns"`
	TCP          []TCPResult `json:"tcp"`   // one per resolved IP
	TLS          []TLSResult `json:"tls"`   // one per resolved IP
	HTTP         *HTTPResult `json:"http"`
	MeasuredAt   time.Time   `json:"measured_at"`
	TotalMS      int64       `json:"total_ms"`
}

// SubmitMeasurementsRequest is sent by the probe node to report a batch of
// completed measurements (POST /probe/v1/measurements).
type SubmitMeasurementsRequest struct {
	NodeID       string        `json:"node_id"`
	Measurements []Measurement `json:"measurements"`
}
