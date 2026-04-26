package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"domain-platform/pkg/provider/aliyunauth"
)

func init() {
	Register("alidns", NewAlidnsProvider)
}

// ── Config / Credentials ──────────────────────────────────────────────────────

// alidnsConfig is parsed from the dns_providers.config JSONB.
// Example: {"domain_name": "example.com"}
type alidnsConfig struct {
	DomainName string `json:"domain_name"`
}

// alidnsCreds is parsed from the dns_providers.credentials JSONB.
// Example: {"access_key_id": "LTAI5t...", "access_key_secret": "..."}
type alidnsCreds struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
}

// ── Provider ──────────────────────────────────────────────────────────────────

const (
	alidnsBaseURL    = "https://alidns.aliyuncs.com"
	alidnsAPIVersion = "2015-01-09"
	alidnsPageSize   = 500
)

type alidnsProvider struct {
	domainName string // default zone (domain name)
	signer     *aliyunauth.Signer
	baseURL    string
	client     *http.Client
}

// NewAlidnsProvider creates an Aliyun DNS provider from config and credentials JSON.
func NewAlidnsProvider(config, credentials json.RawMessage) (Provider, error) {
	var cfg alidnsConfig
	if err := json.Unmarshal(config, &cfg); err != nil || strings.TrimSpace(cfg.DomainName) == "" {
		return nil, fmt.Errorf("%w: domain_name required in config", ErrMissingConfig)
	}
	var creds alidnsCreds
	if err := json.Unmarshal(credentials, &creds); err != nil ||
		strings.TrimSpace(creds.AccessKeyID) == "" || strings.TrimSpace(creds.AccessKeySecret) == "" {
		return nil, fmt.Errorf("%w: access_key_id and access_key_secret required", ErrMissingCredentials)
	}

	return &alidnsProvider{
		domainName: cfg.DomainName,
		signer:     aliyunauth.New(creds.AccessKeyID, creds.AccessKeySecret),
		baseURL:    alidnsBaseURL,
		client:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// newAlidnsProviderWithClient allows injecting a custom HTTP client and base URL.
// Used in tests to point at an httptest.Server instead of the real API.
func newAlidnsProviderWithClient(domainName, keyID, keySecret, baseURL string, client *http.Client) Provider {
	return &alidnsProvider{
		domainName: domainName,
		signer:     aliyunauth.New(keyID, keySecret),
		baseURL:    baseURL,
		client:     client,
	}
}

func (p *alidnsProvider) Name() string { return "alidns" }

// ── Wire types ────────────────────────────────────────────────────────────────

// alidnsRecordItem mirrors the Aliyun DNS DescribeDomainRecords record shape.
type alidnsRecordItem struct {
	RecordId   string `json:"RecordId"`
	RR         string `json:"RR"`      // subdomain part: "www", "@", etc.
	Type       string `json:"Type"`
	Value      string `json:"Value"`
	TTL        int    `json:"TTL"`
	Priority   int    `json:"Priority"` // MX only; 0 for other types
	DomainName string `json:"DomainName"`
}

type alidnsRecordsData struct {
	Record     []alidnsRecordItem `json:"Record"`
	TotalCount int                `json:"TotalCount"`
	PageNumber int                `json:"PageNumber"`
	PageSize   int                `json:"PageSize"`
}

type alidnsListResponse struct {
	RequestId     string            `json:"RequestId"`
	DomainRecords alidnsRecordsData `json:"DomainRecords"`
}

type alidnsAddRecordResponse struct {
	RequestId string `json:"RequestId"`
	RecordId  string `json:"RecordId"`
}

type alidnsDomainInfoResponse struct {
	RequestId  string `json:"RequestId"`
	DomainInfo struct {
		DomainName string `json:"DomainName"`
		DnsServers struct {
			DnsServer []string `json:"DnsServer"`
		} `json:"DnsServers"`
	} `json:"DomainInfo"`
}

// alidnsError is the error envelope returned by Aliyun DNS APIs.
type alidnsError struct {
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	RequestId string `json:"RequestId"`
}

// ── Zone resolution ───────────────────────────────────────────────────────────

// resolveZone returns the domain name to use. In Aliyun DNS the zone is always
// a plain domain name (not an ID), so we just fall back to config if empty.
func (p *alidnsProvider) resolveZone(zone string) string {
	if zone == "" {
		return p.domainName
	}
	return zone
}

// ── Record name helpers ───────────────────────────────────────────────────────

// rrFromName extracts the Aliyun RR (relative record name) from a fully
// qualified record name and its zone. Examples:
//
//	("www.example.com", "example.com") → "www"
//	("example.com",     "example.com") → "@"
//	("sub.www.example.com", "example.com") → "sub.www"
func rrFromName(fqdn, zone string) string {
	fqdn = strings.TrimSuffix(strings.ToLower(fqdn), ".")
	zone = strings.TrimSuffix(strings.ToLower(zone), ".")
	if fqdn == zone {
		return "@"
	}
	suffix := "." + zone
	if strings.HasSuffix(fqdn, suffix) {
		return strings.TrimSuffix(fqdn, suffix)
	}
	// Caller passed just a subdomain label without the zone suffix.
	return fqdn
}

// nameFromRR reconstructs the fully qualified name from RR and zone.
func nameFromRR(rr, zone string) string {
	if rr == "@" || rr == "" {
		return zone
	}
	return rr + "." + zone
}

// ── Record conversion ─────────────────────────────────────────────────────────

// alidnsToRecord converts an Aliyun DNS record item to our provider-agnostic
// Record type.
//
// Special cases:
//   - MX: Priority is a separate field in the API (not part of Value).
//   - TXT: Aliyun may return the value wrapped in double quotes; we strip them.
//   - SRV: Value is "priority weight port target"; we parse into Extra.
func alidnsToRecord(r alidnsRecordItem) Record {
	rec := Record{
		ID:       r.RecordId,
		Type:     r.Type,
		Name:     nameFromRR(r.RR, r.DomainName),
		Content:  r.Value,
		TTL:      r.TTL,
		Priority: r.Priority,
	}

	switch r.Type {
	case RecordTypeTXT:
		// Strip surrounding double-quotes if present (Aliyun API quirk).
		rec.Content = strings.Trim(r.Value, `"`)
	case RecordTypeSRV:
		// Value format: "priority weight port target"
		parts := strings.Fields(r.Value)
		if len(parts) == 4 {
			if prio, err := strconv.Atoi(parts[0]); err == nil {
				rec.Priority = prio
			}
			rec.Extra = map[string]string{
				"weight": parts[1],
				"port":   parts[2],
				"target": parts[3],
			}
		}
	}

	return rec
}

// ── List ──────────────────────────────────────────────────────────────────────

func (p *alidnsProvider) ListRecords(ctx context.Context, zone string, filter RecordFilter) ([]Record, error) {
	domain := p.resolveZone(zone)
	var all []Record
	pageNum := 1

	for {
		params := map[string]string{
			"Action":     "DescribeDomainRecords",
			"DomainName": domain,
			"PageNumber": strconv.Itoa(pageNum),
			"PageSize":   strconv.Itoa(alidnsPageSize),
		}
		if filter.Type != "" {
			params["TypeKeyWord"] = filter.Type
		}
		if filter.Name != "" {
			// Aliyun filter is on RR (subdomain), not full FQDN.
			params["RRKeyWord"] = rrFromName(filter.Name, domain)
		}

		body, err := p.doRequest(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("alidns list records: %w", err)
		}

		var resp alidnsListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("alidns parse list response: %w", err)
		}

		for _, r := range resp.DomainRecords.Record {
			all = append(all, alidnsToRecord(r))
		}

		// Aliyun DNS uses page-based pagination; stop when we've fetched all pages.
		fetched := (pageNum-1)*alidnsPageSize + len(resp.DomainRecords.Record)
		if fetched >= resp.DomainRecords.TotalCount || len(resp.DomainRecords.Record) == 0 {
			break
		}
		pageNum++
	}

	return all, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

func (p *alidnsProvider) CreateRecord(ctx context.Context, zone string, record Record) (*Record, error) {
	domain := p.resolveZone(zone)
	rr := rrFromName(record.Name, domain)
	value := alidnsBuildValue(record)

	params := map[string]string{
		"Action":     "AddDomainRecord",
		"DomainName": domain,
		"RR":         rr,
		"Type":       record.Type,
		"Value":      value,
		"TTL":        strconv.Itoa(record.TTL),
	}
	if record.Priority > 0 {
		params["Priority"] = strconv.Itoa(record.Priority)
	}

	body, err := p.doRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("alidns create record: %w", err)
	}

	var resp alidnsAddRecordResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("alidns parse create response: %w", err)
	}

	out := record
	out.ID = resp.RecordId
	out.Name = nameFromRR(rr, domain)
	out.Content = record.Content // keep original (not the wire value)
	return &out, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (p *alidnsProvider) UpdateRecord(ctx context.Context, zone string, recordID string, record Record) (*Record, error) {
	domain := p.resolveZone(zone)
	rr := rrFromName(record.Name, domain)
	value := alidnsBuildValue(record)

	params := map[string]string{
		"Action":   "UpdateDomainRecord",
		"RecordId": recordID,
		"RR":       rr,
		"Type":     record.Type,
		"Value":    value,
		"TTL":      strconv.Itoa(record.TTL),
	}
	if record.Priority > 0 {
		params["Priority"] = strconv.Itoa(record.Priority)
	}

	body, err := p.doRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("alidns update record: %w", err)
	}

	// UpdateDomainRecord returns {"RequestId":"...", "RecordId":"..."}
	var resp alidnsAddRecordResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("alidns parse update response: %w", err)
	}

	out := record
	out.ID = recordID
	out.Name = nameFromRR(rr, domain)
	return &out, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (p *alidnsProvider) DeleteRecord(ctx context.Context, zone string, recordID string) error {
	params := map[string]string{
		"Action":   "DeleteDomainRecord",
		"RecordId": recordID,
	}

	_, err := p.doRequest(ctx, params)
	if err != nil {
		return fmt.Errorf("alidns delete record: %w", err)
	}
	return nil
}

// ── GetNameservers ────────────────────────────────────────────────────────────

func (p *alidnsProvider) GetNameservers(ctx context.Context, zone string) ([]string, error) {
	domain := p.resolveZone(zone)

	params := map[string]string{
		"Action":     "DescribeDomainInfo",
		"DomainName": domain,
	}

	body, err := p.doRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("alidns get nameservers: %w", err)
	}

	var resp alidnsDomainInfoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("alidns parse domain info: %w", err)
	}

	ns := resp.DomainInfo.DnsServers.DnsServer
	if len(ns) == 0 {
		return nil, fmt.Errorf("%w: no nameservers returned for %s", ErrZoneNotFound, domain)
	}
	return ns, nil
}

// ── BatchCreateRecords ────────────────────────────────────────────────────────

// BatchCreateRecords creates records sequentially (Aliyun DNS has no batch
// create API). On the first failure it returns the records created so far.
func (p *alidnsProvider) BatchCreateRecords(ctx context.Context, zone string, records []Record) ([]Record, error) {
	created := make([]Record, 0, len(records))
	for _, rec := range records {
		r, err := p.CreateRecord(ctx, zone, rec)
		if err != nil {
			return created, fmt.Errorf("batch create %s %s: %w", rec.Type, rec.Name, err)
		}
		created = append(created, *r)
	}
	return created, nil
}

// ── BatchDeleteRecords ────────────────────────────────────────────────────────

// BatchDeleteRecords deletes records by ID sequentially.
func (p *alidnsProvider) BatchDeleteRecords(ctx context.Context, zone string, recordIDs []string) error {
	for _, id := range recordIDs {
		if err := p.DeleteRecord(ctx, zone, id); err != nil {
			return fmt.Errorf("batch delete record %s: %w", id, err)
		}
	}
	return nil
}

// ── Value builder ─────────────────────────────────────────────────────────────

// alidnsBuildValue converts a Record's content to the wire Value for Aliyun DNS.
//
// Special cases:
//   - TXT: Aliyun requires the value WITHOUT outer double-quotes in the request.
//   - SRV: build "priority weight port target" from Priority + Extra.
//   - All others: pass Content as-is.
func alidnsBuildValue(rec Record) string {
	switch rec.Type {
	case RecordTypeTXT:
		// Strip quotes if caller accidentally included them.
		return strings.Trim(rec.Content, `"`)
	case RecordTypeSRV:
		weight := rec.Extra["weight"]
		port := rec.Extra["port"]
		target := rec.Extra["target"]
		return fmt.Sprintf("%d %s %s %s", rec.Priority, weight, port, target)
	default:
		return rec.Content
	}
}

// ── HTTP + signing ────────────────────────────────────────────────────────────

// doRequest builds, signs, and executes a GET request to the Aliyun DNS API.
func (p *alidnsProvider) doRequest(ctx context.Context, params map[string]string) ([]byte, error) {
	full := p.signer.CommonParams(alidnsAPIVersion)
	for k, v := range params {
		full[k] = v
	}

	rawURL := p.signer.SignedURL(p.baseURL, full)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("alidns build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alidns request: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err := alidnsCheckStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}
	return body, nil
}

// ── Error mapping ─────────────────────────────────────────────────────────────

// alidnsCheckStatus maps Aliyun DNS HTTP status codes and API error codes to
// typed sentinel errors. Aliyun sometimes returns errors with HTTP 200; we
// inspect the body in that case.
func alidnsCheckStatus(code int, body []byte) error {
	if code == http.StatusOK {
		var apiErr alidnsError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Code != "" {
			return alidnsMapCode(apiErr.Code, apiErr.Message)
		}
		return nil
	}

	var apiErr alidnsError
	_ = json.Unmarshal(body, &apiErr)

	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		if apiErr.Code != "" {
			return alidnsMapCode(apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, code)
	case http.StatusNotFound:
		return fmt.Errorf("%w: HTTP 404", ErrRecordNotFound)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w", ErrRateLimitExceeded)
	default:
		if apiErr.Code != "" {
			return alidnsMapCode(apiErr.Code, apiErr.Message)
		}
		msg := string(body)
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return fmt.Errorf("alidns HTTP %d: %s", code, msg)
	}
}

// alidnsMapCode translates an Aliyun DNS error code to a typed sentinel error.
func alidnsMapCode(code, message string) error {
	switch code {
	case "InvalidAccessKeyId.NotFound", "InvalidAccessKeyId", "SignatureDoesNotMatch",
		"InvalidAccessKeySecret":
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	case "DomainRecordNotBelongToUser", "DomainRecordDuplicate":
		return fmt.Errorf("%w: %s", ErrRecordNotFound, message)
	case "RecordForbidden.DNSChange":
		return fmt.Errorf("%w: %s", ErrRecordAlreadyExists, message)
	case "InvalidDomainName.NoExist":
		return fmt.Errorf("%w: %s", ErrZoneNotFound, message)
	case "Throttling", "ServiceUnavailableTemporary", "Throttling.User":
		return fmt.Errorf("%w: %s", ErrRateLimitExceeded, message)
	default:
		return fmt.Errorf("alidns API error %s: %s", code, message)
	}
}
