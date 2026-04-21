package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() {
	Register("cloudflare", NewCloudflareProvider)
}

// ── Config / Credentials ─────────────────────────────────────────────────────

// cloudflareConfig is parsed from the dns_providers.config JSONB.
// Example: {"zone_id": "abc123"}
type cloudflareConfig struct {
	ZoneID string `json:"zone_id"`
}

// cloudflareCreds is parsed from the dns_providers.credentials JSONB.
// Example: {"api_token": "Bearer-token-here"}
type cloudflareCreds struct {
	APIToken string `json:"api_token"`
}

// ── Provider ─────────────────────────────────────────────────────────────────

const cloudflareBaseURL = "https://api.cloudflare.com/client/v4"

type cloudflareProvider struct {
	zoneID   string
	apiToken string
	client   *http.Client
}

// NewCloudflareProvider creates a Cloudflare DNS provider from config and credentials JSON.
func NewCloudflareProvider(config, credentials json.RawMessage) (Provider, error) {
	var cfg cloudflareConfig
	if err := json.Unmarshal(config, &cfg); err != nil || cfg.ZoneID == "" {
		return nil, fmt.Errorf("%w: zone_id required in config", ErrMissingConfig)
	}
	var creds cloudflareCreds
	if err := json.Unmarshal(credentials, &creds); err != nil || creds.APIToken == "" {
		return nil, fmt.Errorf("%w: api_token required in credentials", ErrMissingCredentials)
	}

	return &cloudflareProvider{
		zoneID:   cfg.ZoneID,
		apiToken: creds.APIToken,
		client:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (p *cloudflareProvider) Name() string { return "cloudflare" }

// ── List ──────────────────────────────────────────────────────────────────────

// cloudflareRecord mirrors the Cloudflare API response structure.
type cloudflareRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
	Proxied  bool   `json:"proxied"`
}

type cloudflareListResponse struct {
	Success bool               `json:"success"`
	Errors  []cloudflareError  `json:"errors"`
	Result  []cloudflareRecord `json:"result"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (p *cloudflareProvider) ListRecords(ctx context.Context, zone string, filter RecordFilter) ([]Record, error) {
	if zone == "" {
		zone = p.zoneID
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records?per_page=500", cloudflareBaseURL, zone)
	if filter.Name != "" {
		url += "&name=" + filter.Name
	}
	if filter.Type != "" {
		url += "&type=" + filter.Type
	}

	body, err := p.doGet(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("cloudflare list records: %w", err)
	}

	var resp cloudflareListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare parse response: %w", err)
	}
	if !resp.Success {
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare API error: %s", resp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare API returned success=false")
	}

	records := make([]Record, len(resp.Result))
	for i, r := range resp.Result {
		rec := Record{
			ID:      r.ID,
			Type:    r.Type,
			Name:    r.Name,
			Content: r.Content,
			TTL:     r.TTL,
			Proxied: r.Proxied,
		}
		if r.Priority != nil {
			rec.Priority = *r.Priority
		}
		records[i] = rec
	}
	return records, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type cloudflareCreateRequest struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"`
	Proxied  bool   `json:"proxied"`
}

type cloudflareSingleResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
	Result  cloudflareRecord  `json:"result"`
}

func (p *cloudflareProvider) CreateRecord(ctx context.Context, zone string, record Record) (*Record, error) {
	if zone == "" {
		zone = p.zoneID
	}

	payload := cloudflareCreateRequest{
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Priority: record.Priority,
		Proxied:  record.Proxied,
	}
	data, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareBaseURL, zone)
	body, err := p.doPost(ctx, url, data)
	if err != nil {
		return nil, fmt.Errorf("cloudflare create record: %w", err)
	}

	var resp cloudflareSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare parse create response: %w", err)
	}
	if !resp.Success {
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare create error: %s", resp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare create returned success=false")
	}

	r := resp.Result
	out := &Record{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: r.TTL, Proxied: r.Proxied,
	}
	if r.Priority != nil {
		out.Priority = *r.Priority
	}
	return out, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (p *cloudflareProvider) UpdateRecord(ctx context.Context, zone string, recordID string, record Record) (*Record, error) {
	if zone == "" {
		zone = p.zoneID
	}

	payload := cloudflareCreateRequest{
		Type:     record.Type,
		Name:     record.Name,
		Content:  record.Content,
		TTL:      record.TTL,
		Priority: record.Priority,
		Proxied:  record.Proxied,
	}
	data, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, zone, recordID)
	body, err := p.doPut(ctx, url, data)
	if err != nil {
		return nil, fmt.Errorf("cloudflare update record: %w", err)
	}

	var resp cloudflareSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare parse update response: %w", err)
	}
	if !resp.Success {
		if len(resp.Errors) > 0 {
			return nil, fmt.Errorf("cloudflare update error: %s", resp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare update returned success=false")
	}

	r := resp.Result
	out := &Record{
		ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content,
		TTL: r.TTL, Proxied: r.Proxied,
	}
	if r.Priority != nil {
		out.Priority = *r.Priority
	}
	return out, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (p *cloudflareProvider) DeleteRecord(ctx context.Context, zone string, recordID string) error {
	if zone == "" {
		zone = p.zoneID
	}

	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, zone, recordID)
	body, err := p.doDelete(ctx, url)
	if err != nil {
		return fmt.Errorf("cloudflare delete record: %w", err)
	}

	var resp cloudflareSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("cloudflare parse delete response: %w", err)
	}
	if !resp.Success {
		if len(resp.Errors) > 0 {
			return fmt.Errorf("cloudflare delete error: %s", resp.Errors[0].Message)
		}
	}
	return nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (p *cloudflareProvider) doGet(ctx context.Context, url string) ([]byte, error) {
	return p.doRequest(ctx, http.MethodGet, url, nil)
}

func (p *cloudflareProvider) doPost(ctx context.Context, url string, body []byte) ([]byte, error) {
	return p.doRequest(ctx, http.MethodPost, url, body)
}

func (p *cloudflareProvider) doPut(ctx context.Context, url string, body []byte) ([]byte, error) {
	return p.doRequest(ctx, http.MethodPut, url, body)
}

func (p *cloudflareProvider) doDelete(ctx context.Context, url string) ([]byte, error) {
	return p.doRequest(ctx, http.MethodDelete, url, nil)
}

func (p *cloudflareProvider) doRequest(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}
