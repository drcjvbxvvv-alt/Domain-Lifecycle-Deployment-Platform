package registrar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"domain-platform/pkg/provider/aliyunauth"
)

func init() {
	Register("aliyun", newAliyunProvider)
}

// ── Credentials ────────────────────────────────────────────────────────────────

// AliyunCredentials is the expected shape of registrar_accounts.credentials
// for Aliyun (阿里雲萬網) accounts.
//
// JSON example:
//
//	{ "access_key_id": "LTAI5t...", "access_key_secret": "..." }
type AliyunCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
}

// ── Provider ───────────────────────────────────────────────────────────────────

type aliyunProvider struct {
	signer  *aliyunauth.Signer
	baseURL string
	client  *http.Client
}

const (
	aliyunDefaultBaseURL = "https://domain.aliyuncs.com"
	aliyunAPIVersion     = "2018-01-29"
	aliyunDateFormat     = "2006-01-02 15:04:05"
	aliyunPageSize       = 100
)

func newAliyunProvider(credentials json.RawMessage) (Provider, error) {
	var creds AliyunCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMissingCredentials, err)
	}
	if strings.TrimSpace(creds.AccessKeyID) == "" || strings.TrimSpace(creds.AccessKeySecret) == "" {
		return nil, fmt.Errorf("%w: access_key_id and access_key_secret are required", ErrMissingCredentials)
	}

	return &aliyunProvider{
		signer:  aliyunauth.New(creds.AccessKeyID, creds.AccessKeySecret),
		baseURL: aliyunDefaultBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// newAliyunProviderWithClient allows injecting a custom HTTP client and base URL
// for testing.
func newAliyunProviderWithClient(creds AliyunCredentials, baseURL string, client *http.Client) Provider {
	return &aliyunProvider{
		signer:  aliyunauth.New(creds.AccessKeyID, creds.AccessKeySecret),
		baseURL: baseURL,
		client:  client,
	}
}

func (p *aliyunProvider) Name() string { return "aliyun" }

// ── Wire types (Aliyun API response shapes) ────────────────────────────────────

type aliyunDomainItem struct {
	DomainName       string `json:"DomainName"`
	RegistrationDate string `json:"RegistrationDate"`
	ExpirationDate   string `json:"ExpirationDate"`
	AutoRenew        bool   `json:"AutoRenew"`
}

type aliyunDomainListData struct {
	Domain         []aliyunDomainItem `json:"Domain"`
	TotalItemNum   int                `json:"TotalItemNum"`
	CurrentPageNum int                `json:"CurrentPageNum"`
	NextPage       bool               `json:"NextPage"`
	PageSize       int                `json:"PageSize"`
}

type aliyunDomainListResponse struct {
	RequestId string               `json:"RequestId"`
	Data      aliyunDomainListData `json:"Data"`
}

type aliyunSingleDomainData struct {
	DomainName       string `json:"DomainName"`
	RegistrationDate string `json:"RegistrationDate"`
	ExpirationDate   string `json:"ExpirationDate"`
	AutoRenew        bool   `json:"AutoRenew"`
}

// aliyunErrorResponse is the JSON shape for Aliyun API errors.
type aliyunErrorResponse struct {
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	RequestId string `json:"RequestId"`
}

// ── ListDomains ────────────────────────────────────────────────────────────────

// ListDomains fetches all domains in the account using page-based pagination.
func (p *aliyunProvider) ListDomains(ctx context.Context) ([]DomainInfo, error) {
	var all []DomainInfo
	pageNum := 1

	for {
		params := map[string]string{
			"Action":   "QueryDomainList",
			"PageNum":  fmt.Sprintf("%d", pageNum),
			"PageSize": fmt.Sprintf("%d", aliyunPageSize),
		}

		body, err := p.doRequest(ctx, params)
		if err != nil {
			return nil, err
		}

		var resp aliyunDomainListResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("aliyun parse domain list: %w", err)
		}

		for _, d := range resp.Data.Domain {
			all = append(all, aliyunToDomainInfo(d))
		}

		if !resp.Data.NextPage {
			break
		}
		pageNum++
	}

	return all, nil
}

// ── GetDomain ──────────────────────────────────────────────────────────────────

func (p *aliyunProvider) GetDomain(ctx context.Context, fqdn string) (*DomainInfo, error) {
	params := map[string]string{
		"Action":     "QueryDomainByDomainName",
		"DomainName": strings.ToLower(strings.TrimSpace(fqdn)),
	}

	body, err := p.doRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	// QueryDomainByDomainName returns the domain object at the top level.
	var d aliyunSingleDomainData
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("aliyun parse domain: %w", err)
	}

	if strings.TrimSpace(d.DomainName) == "" {
		return nil, ErrDomainNotFound
	}

	info := aliyunToDomainInfo(aliyunDomainItem{
		DomainName:       d.DomainName,
		RegistrationDate: d.RegistrationDate,
		ExpirationDate:   d.ExpirationDate,
		AutoRenew:        d.AutoRenew,
	})
	return &info, nil
}

// ── HTTP + Signing ─────────────────────────────────────────────────────────────

// doRequest builds, signs, and executes a GET request to the Aliyun Domain API.
// It maps Aliyun error codes to typed sentinel errors.
func (p *aliyunProvider) doRequest(ctx context.Context, params map[string]string) ([]byte, error) {
	// Merge common params with action-specific params.
	full := p.signer.CommonParams(aliyunAPIVersion)
	for k, v := range params {
		full[k] = v
	}

	rawURL := p.signer.SignedURL(p.baseURL, full)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("aliyun build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aliyun request: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err := aliyunCheckStatus(resp.StatusCode, body); err != nil {
		return nil, err
	}

	return body, nil
}

// ── Error mapping ──────────────────────────────────────────────────────────────

// aliyunCheckStatus maps Aliyun HTTP status codes and error codes to typed errors.
func aliyunCheckStatus(code int, body []byte) error {
	if code == http.StatusOK {
		// Even on 200 the body might carry an error envelope
		var apiErr aliyunErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Code != "" {
			return aliyunMapCode(apiErr.Code, apiErr.Message)
		}
		return nil
	}

	// Parse error body for non-200 responses
	var apiErr aliyunErrorResponse
	_ = json.Unmarshal(body, &apiErr)

	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		if apiErr.Code != "" {
			return aliyunMapCode(apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, code)
	case http.StatusNotFound:
		return ErrDomainNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimitExceeded
	default:
		return fmt.Errorf("aliyun API error %d: %s", code, truncate(string(body), 200))
	}
}

// aliyunMapCode translates an Aliyun error code string into a typed sentinel error.
func aliyunMapCode(code, message string) error {
	switch code {
	case "InvalidAccessKeyId.NotFound", "InvalidAccessKeyId", "SignatureDoesNotMatch":
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	case "Forbidden.RAM":
		return fmt.Errorf("%w: %s", ErrAccessDenied, message)
	case "Throttling", "ServiceUnavailableTemporary":
		return fmt.Errorf("%w: %s", ErrRateLimitExceeded, message)
	default:
		return fmt.Errorf("aliyun API error %s: %s", code, message)
	}
}

// ── Conversion helpers ─────────────────────────────────────────────────────────

func aliyunToDomainInfo(d aliyunDomainItem) DomainInfo {
	info := DomainInfo{
		FQDN:      strings.ToLower(strings.TrimSpace(d.DomainName)),
		AutoRenew: d.AutoRenew,
	}

	// Aliyun returns dates as "2006-01-02 15:04:05" (Beijing time, parsed as UTC)
	if t, err := time.Parse(aliyunDateFormat, d.RegistrationDate); err == nil {
		info.RegistrationDate = &t
	}
	if t, err := time.Parse(aliyunDateFormat, d.ExpirationDate); err == nil {
		info.ExpiryDate = &t
	}

	return info
}
