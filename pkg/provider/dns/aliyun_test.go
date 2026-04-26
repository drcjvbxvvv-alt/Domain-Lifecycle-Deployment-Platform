package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── test helpers ──────────────────────────────────────────────────────────────

const (
	alidnsTestDomain    = "example.com"
	alidnsTestKeyID     = "test-key-id"
	alidnsTestKeySecret = "test-key-secret"
)

func newAlidnsTestProvider(t *testing.T, handler http.Handler) (Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := newAlidnsProviderWithClient(alidnsTestDomain, alidnsTestKeyID, alidnsTestKeySecret, srv.URL, srv.Client())
	return p, srv
}

// buildAlidnsListResp builds an Aliyun DescribeDomainRecords response.
func buildAlidnsListResp(records []alidnsRecordItem, total, page int) []byte {
	resp := alidnsListResponse{
		DomainRecords: alidnsRecordsData{
			Record:     records,
			TotalCount: total,
			PageNumber: page,
			PageSize:   alidnsPageSize,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// buildAlidnsAddResp builds an AddDomainRecord response.
func buildAlidnsAddResp(recordID string) []byte {
	b, _ := json.Marshal(alidnsAddRecordResponse{RecordId: recordID, RequestId: "test-req"})
	return b
}

// buildAlidnsNSResp builds a DescribeDomainInfo response with nameservers.
func buildAlidnsNSResp(ns []string) []byte {
	resp := alidnsDomainInfoResponse{}
	resp.DomainInfo.DomainName = alidnsTestDomain
	resp.DomainInfo.DnsServers.DnsServer = ns
	b, _ := json.Marshal(resp)
	return b
}

// buildAlidnsError builds an Aliyun-style error body.
func buildAlidnsError(code, message string) []byte {
	b, _ := json.Marshal(alidnsError{Code: code, Message: message, RequestId: "test"})
	return b
}

// assertAction checks that the mock received the expected Action param.
func assertAction(t *testing.T, r *http.Request, want string) {
	t.Helper()
	got := r.URL.Query().Get("Action")
	assert.Equal(t, want, got, "expected Action=%s", want)
}

// ── NewAlidnsProvider ─────────────────────────────────────────────────────────

func TestNewAlidnsProvider_Valid(t *testing.T) {
	p, err := NewAlidnsProvider(
		json.RawMessage(`{"domain_name":"example.com"}`),
		json.RawMessage(`{"access_key_id":"kid","access_key_secret":"ksec"}`),
	)
	require.NoError(t, err)
	assert.Equal(t, "alidns", p.Name())
}

func TestNewAlidnsProvider_MissingDomain(t *testing.T) {
	_, err := NewAlidnsProvider(
		json.RawMessage(`{}`),
		json.RawMessage(`{"access_key_id":"kid","access_key_secret":"ksec"}`),
	)
	assert.ErrorIs(t, err, ErrMissingConfig)
}

func TestNewAlidnsProvider_MissingCredentials(t *testing.T) {
	tests := []struct {
		name  string
		creds string
	}{
		{"empty", `{}`},
		{"missing_secret", `{"access_key_id":"kid"}`},
		{"missing_key_id", `{"access_key_secret":"ksec"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAlidnsProvider(
				json.RawMessage(`{"domain_name":"example.com"}`),
				json.RawMessage(tt.creds),
			)
			assert.ErrorIs(t, err, ErrMissingCredentials)
		})
	}
}

// ── resolveZone / rrFromName / nameFromRR ─────────────────────────────────────

func TestRRFromName(t *testing.T) {
	tests := []struct {
		fqdn, zone, want string
	}{
		{"example.com", "example.com", "@"},
		{"www.example.com", "example.com", "www"},
		{"sub.www.example.com", "example.com", "sub.www"},
		{"example.com.", "example.com", "@"},        // trailing dot stripped
		{"www.example.com.", "example.com.", "www"}, // both trailing dots
		{"shop.example.com", "example.com", "shop"},
	}
	for _, tt := range tests {
		t.Run(tt.fqdn, func(t *testing.T) {
			assert.Equal(t, tt.want, rrFromName(tt.fqdn, tt.zone))
		})
	}
}

func TestNameFromRR(t *testing.T) {
	assert.Equal(t, "example.com", nameFromRR("@", "example.com"))
	assert.Equal(t, "example.com", nameFromRR("", "example.com"))
	assert.Equal(t, "www.example.com", nameFromRR("www", "example.com"))
	assert.Equal(t, "sub.www.example.com", nameFromRR("sub.www", "example.com"))
}

// ── ListRecords ───────────────────────────────────────────────────────────────

func TestAlidns_ListRecords_HappyPath(t *testing.T) {
	records := []alidnsRecordItem{
		{RecordId: "r1", RR: "@", Type: "A", Value: "1.2.3.4", TTL: 600, DomainName: alidnsTestDomain},
		{RecordId: "r2", RR: "www", Type: "CNAME", Value: "example.com", TTL: 300, DomainName: alidnsTestDomain},
	}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "DescribeDomainRecords")
		assert.Equal(t, alidnsTestDomain, r.URL.Query().Get("DomainName"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(records, 2, 1))
	}))

	got, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "r1", got[0].ID)
	assert.Equal(t, "A", got[0].Type)
	assert.Equal(t, "example.com", got[0].Name) // RR="@" → zone
	assert.Equal(t, "1.2.3.4", got[0].Content)

	assert.Equal(t, "r2", got[1].ID)
	assert.Equal(t, "www.example.com", got[1].Name)
}

func TestAlidns_ListRecords_UsesDefaultZone(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, alidnsTestDomain, r.URL.Query().Get("DomainName"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(nil, 0, 1))
	}))

	_, err := p.ListRecords(context.Background(), "", RecordFilter{})
	require.NoError(t, err)
}

func TestAlidns_ListRecords_FilterType(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "MX", r.URL.Query().Get("TypeKeyWord"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(nil, 0, 1))
	}))

	_, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{Type: "MX"})
	require.NoError(t, err)
}

func TestAlidns_ListRecords_FilterName(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "www.example.com" filtered by RR → "www"
		assert.Equal(t, "www", r.URL.Query().Get("RRKeyWord"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(nil, 0, 1))
	}))

	_, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{Name: "www.example.com"})
	require.NoError(t, err)
}

func TestAlidns_ListRecords_Pagination(t *testing.T) {
	// 3 records split across 2 pages (pageSize=500 so this simulates TotalCount > first-page count)
	page1Records := make([]alidnsRecordItem, 500)
	for i := 0; i < 500; i++ {
		page1Records[i] = alidnsRecordItem{
			RecordId: fmt.Sprintf("r%d", i+1), RR: fmt.Sprintf("h%d", i+1),
			Type: "A", Value: "1.1.1.1", TTL: 300, DomainName: alidnsTestDomain,
		}
	}
	page2Records := []alidnsRecordItem{
		{RecordId: "r501", RR: "h501", Type: "A", Value: "1.1.1.1", TTL: 300, DomainName: alidnsTestDomain},
	}

	call := 0
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		page, _ := strconv.Atoi(r.URL.Query().Get("PageNumber"))
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			_, _ = w.Write(buildAlidnsListResp(page1Records, 501, 1))
		} else {
			_, _ = w.Write(buildAlidnsListResp(page2Records, 501, 2))
		}
	}))

	got, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 501)
	assert.Equal(t, 2, call, "should have made 2 pages of requests")
}

func TestAlidns_ListRecords_TXT_QuotesStripped(t *testing.T) {
	records := []alidnsRecordItem{
		{RecordId: "txt1", RR: "@", Type: "TXT", Value: `"v=spf1 include:example.com ~all"`, TTL: 300, DomainName: alidnsTestDomain},
	}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(records, 1, 1))
	}))

	got, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	// Outer quotes stripped
	assert.Equal(t, "v=spf1 include:example.com ~all", got[0].Content)
}

func TestAlidns_ListRecords_MX_Priority(t *testing.T) {
	records := []alidnsRecordItem{
		{RecordId: "mx1", RR: "@", Type: "MX", Value: "mail.example.com", TTL: 300, Priority: 10, DomainName: alidnsTestDomain},
	}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(records, 1, 1))
	}))

	got, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 10, got[0].Priority)
	assert.Equal(t, "mail.example.com", got[0].Content)
}

func TestAlidns_ListRecords_SRV_ParsedIntoExtra(t *testing.T) {
	records := []alidnsRecordItem{
		{RecordId: "srv1", RR: "_sip._tcp", Type: "SRV", Value: "10 20 5060 sip.example.com", TTL: 300, DomainName: alidnsTestDomain},
	}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsListResp(records, 1, 1))
	}))

	got, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	srv := got[0]
	assert.Equal(t, 10, srv.Priority)
	assert.Equal(t, "20", srv.Extra["weight"])
	assert.Equal(t, "5060", srv.Extra["port"])
	assert.Equal(t, "sip.example.com", srv.Extra["target"])
}

func TestAlidns_ListRecords_InvalidCredentials(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(buildAlidnsError("InvalidAccessKeyId.NotFound", "access key not found"))
	}))

	_, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestAlidns_ListRecords_RateLimit(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{}`))
	}))

	_, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

func TestAlidns_ListRecords_RateLimitInBody(t *testing.T) {
	// Aliyun sometimes returns 200 with a Throttling code in the body
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buildAlidnsError("Throttling", "Request was denied due to user flow control"))
	}))

	_, err := p.ListRecords(context.Background(), alidnsTestDomain, RecordFilter{})
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

// ── CreateRecord ──────────────────────────────────────────────────────────────

func TestAlidns_CreateRecord_HappyPath(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "AddDomainRecord")
		assert.Equal(t, alidnsTestDomain, r.URL.Query().Get("DomainName"))
		assert.Equal(t, "www", r.URL.Query().Get("RR"))
		assert.Equal(t, "A", r.URL.Query().Get("Type"))
		assert.Equal(t, "5.6.7.8", r.URL.Query().Get("Value"))
		assert.Equal(t, "300", r.URL.Query().Get("TTL"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("new-id"))
	}))

	rec, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: "A", Name: "www.example.com", Content: "5.6.7.8", TTL: 300,
	})
	require.NoError(t, err)
	assert.Equal(t, "new-id", rec.ID)
	assert.Equal(t, "www.example.com", rec.Name)
}

func TestAlidns_CreateRecord_RootRecord(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "AddDomainRecord")
		assert.Equal(t, "@", r.URL.Query().Get("RR"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("root-id"))
	}))

	rec, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: "A", Name: "example.com", Content: "1.2.3.4", TTL: 300,
	})
	require.NoError(t, err)
	assert.Equal(t, "root-id", rec.ID)
}

func TestAlidns_CreateRecord_MX_Priority(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "AddDomainRecord")
		assert.Equal(t, "MX", r.URL.Query().Get("Type"))
		assert.Equal(t, "10", r.URL.Query().Get("Priority"))
		assert.Equal(t, "mail.example.com", r.URL.Query().Get("Value"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("mx-id"))
	}))

	rec, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: "MX", Name: "example.com", Content: "mail.example.com", TTL: 300, Priority: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, "mx-id", rec.ID)
}

func TestAlidns_CreateRecord_TXT_NoExtraQuotes(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Value should NOT have outer quotes when sent to API
		assert.Equal(t, "v=spf1 include:example.com ~all", r.URL.Query().Get("Value"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("txt-id"))
	}))

	_, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: "TXT", Name: "example.com", Content: "v=spf1 include:example.com ~all", TTL: 300,
	})
	require.NoError(t, err)
}

func TestAlidns_CreateRecord_TXT_StripsCallerQuotes(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Even if caller passes quoted content, API receives unquoted
		assert.Equal(t, "v=spf1 ~all", r.URL.Query().Get("Value"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("txt-id"))
	}))

	_, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: "TXT", Name: "example.com", Content: `"v=spf1 ~all"`, TTL: 300,
	})
	require.NoError(t, err)
}

func TestAlidns_CreateRecord_SRV_BuildsValue(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "AddDomainRecord")
		// SRV value: "priority weight port target"
		assert.Equal(t, "10 20 5060 sip.example.com", r.URL.Query().Get("Value"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("srv-id"))
	}))

	_, err := p.CreateRecord(context.Background(), alidnsTestDomain, Record{
		Type: RecordTypeSRV, Name: "_sip._tcp.example.com", TTL: 300, Priority: 10,
		Extra: map[string]string{"weight": "20", "port": "5060", "target": "sip.example.com"},
	})
	require.NoError(t, err)
}

// ── UpdateRecord ──────────────────────────────────────────────────────────────

func TestAlidns_UpdateRecord_HappyPath(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "UpdateDomainRecord")
		assert.Equal(t, "rec-123", r.URL.Query().Get("RecordId"))
		assert.Equal(t, "www", r.URL.Query().Get("RR"))
		assert.Equal(t, "A", r.URL.Query().Get("Type"))
		assert.Equal(t, "9.9.9.9", r.URL.Query().Get("Value"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp("rec-123"))
	}))

	rec, err := p.UpdateRecord(context.Background(), alidnsTestDomain, "rec-123", Record{
		Type: "A", Name: "www.example.com", Content: "9.9.9.9", TTL: 300,
	})
	require.NoError(t, err)
	assert.Equal(t, "rec-123", rec.ID)
}

func TestAlidns_UpdateRecord_InvalidCredentials(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(buildAlidnsError("SignatureDoesNotMatch", "Specified signature is not matched"))
	}))

	_, err := p.UpdateRecord(context.Background(), alidnsTestDomain, "rec-id", Record{
		Type: "A", Name: "www.example.com", Content: "1.1.1.1", TTL: 300,
	})
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// ── DeleteRecord ──────────────────────────────────────────────────────────────

func TestAlidns_DeleteRecord_HappyPath(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "DeleteDomainRecord")
		assert.Equal(t, "del-id", r.URL.Query().Get("RecordId"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RequestId":"req","RecordId":"del-id"}`))
	}))

	err := p.DeleteRecord(context.Background(), alidnsTestDomain, "del-id")
	assert.NoError(t, err)
}

func TestAlidns_DeleteRecord_NotFound(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buildAlidnsError("DomainRecordNotBelongToUser", "record not found"))
	}))

	err := p.DeleteRecord(context.Background(), alidnsTestDomain, "ghost-id")
	assert.ErrorIs(t, err, ErrRecordNotFound)
}

// ── GetNameservers ────────────────────────────────────────────────────────────

func TestAlidns_GetNameservers_HappyPath(t *testing.T) {
	ns := []string{"dns1.alidns.com", "dns2.alidns.com"}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "DescribeDomainInfo")
		assert.Equal(t, alidnsTestDomain, r.URL.Query().Get("DomainName"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsNSResp(ns))
	}))

	got, err := p.GetNameservers(context.Background(), alidnsTestDomain)
	require.NoError(t, err)
	assert.Equal(t, ns, got)
}

func TestAlidns_GetNameservers_FallbackToDefaultDomain(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, alidnsTestDomain, r.URL.Query().Get("DomainName"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsNSResp([]string{"ns1.alidns.com"}))
	}))

	_, err := p.GetNameservers(context.Background(), "")
	require.NoError(t, err)
}

func TestAlidns_GetNameservers_ZoneNotFound(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buildAlidnsError("InvalidDomainName.NoExist", "domain not found"))
	}))

	_, err := p.GetNameservers(context.Background(), alidnsTestDomain)
	assert.ErrorIs(t, err, ErrZoneNotFound)
}

func TestAlidns_GetNameservers_EmptyList(t *testing.T) {
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsNSResp([]string{}))
	}))

	_, err := p.GetNameservers(context.Background(), alidnsTestDomain)
	assert.ErrorIs(t, err, ErrZoneNotFound)
}

// ── BatchCreateRecords ────────────────────────────────────────────────────────

func TestAlidns_BatchCreateRecords_HappyPath(t *testing.T) {
	call := 0
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp(fmt.Sprintf("id-%d", call)))
	}))

	in := []Record{
		{Type: "A", Name: "a.example.com", Content: "1.1.1.1", TTL: 300},
		{Type: "A", Name: "b.example.com", Content: "2.2.2.2", TTL: 300},
	}
	out, err := p.BatchCreateRecords(context.Background(), alidnsTestDomain, in)
	require.NoError(t, err)
	assert.Len(t, out, 2)
	assert.Equal(t, "id-1", out[0].ID)
	assert.Equal(t, "id-2", out[1].ID)
}

func TestAlidns_BatchCreateRecords_PartialFailure(t *testing.T) {
	call := 0
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildAlidnsAddResp(fmt.Sprintf("id-%d", call)))
	}))

	in := []Record{
		{Type: "A", Name: "ok.example.com", Content: "1.1.1.1", TTL: 300},
		{Type: "A", Name: "fail.example.com", Content: "2.2.2.2", TTL: 300},
		{Type: "A", Name: "never.example.com", Content: "3.3.3.3", TTL: 300},
	}
	out, err := p.BatchCreateRecords(context.Background(), alidnsTestDomain, in)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
	assert.Len(t, out, 1)
}

// ── BatchDeleteRecords ────────────────────────────────────────────────────────

func TestAlidns_BatchDeleteRecords_HappyPath(t *testing.T) {
	deleted := []string{}
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAction(t, r, "DeleteDomainRecord")
		deleted = append(deleted, r.URL.Query().Get("RecordId"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RequestId":"req"}`))
	}))

	err := p.BatchDeleteRecords(context.Background(), alidnsTestDomain, []string{"id-1", "id-2", "id-3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"id-1", "id-2", "id-3"}, deleted)
}

func TestAlidns_BatchDeleteRecords_StopsOnFirstError(t *testing.T) {
	call := 0
	p, _ := newAlidnsTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 2 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buildAlidnsError("DomainRecordNotBelongToUser", "not found"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RequestId":"req"}`))
	}))

	err := p.BatchDeleteRecords(context.Background(), alidnsTestDomain, []string{"ok", "missing", "never"})
	assert.ErrorIs(t, err, ErrRecordNotFound)
	assert.Equal(t, 2, call)
}

// ── alidnsBuildValue ─────────────────────────────────────────────────────────

func TestAlidnsBuildValue(t *testing.T) {
	tests := []struct {
		name string
		rec  Record
		want string
	}{
		{"A record", Record{Type: "A", Content: "1.2.3.4"}, "1.2.3.4"},
		{"CNAME record", Record{Type: "CNAME", Content: "target.example.com"}, "target.example.com"},
		{"TXT no quotes", Record{Type: "TXT", Content: "v=spf1 ~all"}, "v=spf1 ~all"},
		{"TXT strip quotes", Record{Type: "TXT", Content: `"v=spf1 ~all"`}, "v=spf1 ~all"},
		{"MX", Record{Type: "MX", Content: "mail.example.com", Priority: 10}, "mail.example.com"},
		{
			"SRV builds value",
			Record{
				Type: "SRV", Priority: 10,
				Extra: map[string]string{"weight": "20", "port": "5060", "target": "sip.example.com"},
			},
			"10 20 5060 sip.example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, alidnsBuildValue(tt.rec))
		})
	}
}

// ── alidnsCheckStatus ─────────────────────────────────────────────────────────

func TestAlidnsCheckStatus(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		body    []byte
		wantErr error
		wantMsg string
	}{
		{"200 ok no error body", http.StatusOK, []byte(`{"RequestId":"r"}`), nil, ""},
		{"200 with error code", http.StatusOK, buildAlidnsError("InvalidAccessKeyId", "bad key"), ErrUnauthorized, ""},
		{"200 throttling in body", http.StatusOK, buildAlidnsError("Throttling.User", "throttled"), ErrRateLimitExceeded, ""},
		{"401 unauthorized", http.StatusUnauthorized, buildAlidnsError("InvalidAccessKeyId.NotFound", "not found"), ErrUnauthorized, ""},
		{"403 forbidden", http.StatusForbidden, []byte(`{}`), ErrUnauthorized, ""},
		{"404 not found", http.StatusNotFound, []byte(`{}`), ErrRecordNotFound, ""},
		{"429 rate limit", http.StatusTooManyRequests, []byte(`{}`), ErrRateLimitExceeded, ""},
		{"500 with code", http.StatusInternalServerError, buildAlidnsError("InternalError", "oops"), nil, "InternalError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := alidnsCheckStatus(tt.code, tt.body)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else if tt.wantMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), tt.wantMsg), "want %q in %q", tt.wantMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ── Registry ──────────────────────────────────────────────────────────────────

func TestRegistry_AlidnsRegistered(t *testing.T) {
	types := RegisteredTypes()
	found := false
	for _, ty := range types {
		if ty == "alidns" {
			found = true
			break
		}
	}
	assert.True(t, found, "alidns should be registered via init()")
}
