package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testZoneID is a 32-char lowercase hex string that passes cfIsZoneID.
// Using a real zone-ID-shaped value avoids triggering the zone-name lookup
// in resolveZone, which keeps mock handlers simple.
const testZoneID = "abcdef0123456789abcdef0123456789"

// ── test helpers ──────────────────────────────────────────────────────────────

func cfProvider(t *testing.T, handler http.Handler) (Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := newCloudflareProviderWithClient(testZoneID, "tok-xyz", srv.URL, srv.Client())
	return p, srv
}

func cfSuccess(v any) []byte {
	b, _ := json.Marshal(map[string]any{"success": true, "errors": nil, "result": v})
	return b
}

func cfError(code int, msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"success": false,
		"errors":  []map[string]any{{"code": code, "message": msg}},
		"result":  nil,
	})
	return b
}

func cfRecordJSON(id, typ, name, content string, ttl int) map[string]any {
	return map[string]any{
		"id": id, "type": typ, "name": name,
		"content": content, "ttl": ttl, "proxied": false,
	}
}

// ── cfIsZoneID ────────────────────────────────────────────────────────────────

func TestCfIsZoneID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"abcdef0123456789abcdef0123456789", true},  // 32 hex lowercase
		{"ABCDEF0123456789ABCDEF0123456789", false}, // uppercase
		{"abcdef0123456789abcdef012345678", false},  // 31 chars
		{"abcdef0123456789abcdef01234567890", false}, // 33 chars
		{"example.com", false},
		{"zone-abc", false},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, cfIsZoneID(tt.in), "cfIsZoneID(%q)", tt.in)
	}
}

// ── resolveZone / zone cache ──────────────────────────────────────────────────

func TestCloudflare_ResolveZone_EmptyFallsBackToConfigured(t *testing.T) {
	// No HTTP handler needed — empty zone always returns provider's zoneID directly.
	p := newCloudflareProviderWithClient(testZoneID, "tok", "http://unused", &http.Client{})
	cp := p.(*cloudflareProvider)
	id, err := cp.resolveZone(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, testZoneID, id)
}

func TestCloudflare_ResolveZone_32HexPassThrough(t *testing.T) {
	p := newCloudflareProviderWithClient(testZoneID, "tok", "http://unused", &http.Client{})
	cp := p.(*cloudflareProvider)
	anotherZoneID := "fedcba9876543210fedcba9876543210"
	id, err := cp.resolveZone(context.Background(), anotherZoneID)
	require.NoError(t, err)
	assert.Equal(t, anotherZoneID, id)
}

func TestCloudflare_ResolveZone_DomainNameLookupsAPI(t *testing.T) {
	resolvedID := "00000000111111110000000011111111"
	calls := 0
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "example.com", r.URL.Query().Get("name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]map[string]any{{"id": resolvedID, "name": "example.com"}}))
	}))

	cp := p.(*cloudflareProvider)
	id, err := cp.resolveZone(context.Background(), "example.com")
	require.NoError(t, err)
	assert.Equal(t, resolvedID, id)
	assert.Equal(t, 1, calls)
}

func TestCloudflare_ZoneIDCache_SecondCallNoHTTPRequest(t *testing.T) {
	resolvedID := "00000000111111110000000011111111"
	apiCalls := 0
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]map[string]any{{"id": resolvedID, "name": "example.com"}}))
	}))

	cp := p.(*cloudflareProvider)
	for i := 0; i < 5; i++ {
		id, err := cp.resolveZone(context.Background(), "example.com")
		require.NoError(t, err)
		assert.Equal(t, resolvedID, id)
	}
	// Only 1 HTTP request despite 5 resolveZone calls
	assert.Equal(t, 1, apiCalls)
}

func TestCloudflare_ResolveZone_DomainNotFound(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]map[string]any{})) // empty result
	}))

	cp := p.(*cloudflareProvider)
	_, err := cp.resolveZone(context.Background(), "notfound.example.com")
	assert.ErrorIs(t, err, ErrZoneNotFound)
}

// ── ListRecords ───────────────────────────────────────────────────────────────

func TestCloudflare_ListRecords_HappyPath(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/zones/"+testZoneID+"/dns_records")
		assert.Equal(t, "Bearer tok-xyz", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]any{
			cfRecordJSON("r1", "A", "example.com", "1.2.3.4", 300),
			cfRecordJSON("r2", "CNAME", "www.example.com", "example.com", 1),
		}))
	}))

	records, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{})
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "r1", records[0].ID)
	assert.Equal(t, "A", records[0].Type)
	assert.Equal(t, "1.2.3.4", records[0].Content)
	assert.Equal(t, "r2", records[1].ID)
	assert.Equal(t, "CNAME", records[1].Type)
}

func TestCloudflare_ListRecords_AllTypes(t *testing.T) {
	prio10 := 10
	records := []map[string]any{
		cfRecordJSON("a1", "A", "example.com", "1.2.3.4", 300),
		cfRecordJSON("aaaa1", "AAAA", "example.com", "::1", 300),
		cfRecordJSON("cname1", "CNAME", "www.example.com", "example.com", 1),
		cfRecordJSON("txt1", "TXT", "example.com", "v=spf1 include:example.com ~all", 300),
		{"id": "mx1", "type": "MX", "name": "example.com", "content": "mail.example.com", "ttl": 300, "priority": prio10, "proxied": false},
		cfRecordJSON("ns1", "NS", "sub.example.com", "ns1.example.com", 3600),
		cfRecordJSON("ptr1", "PTR", "1.3.2.1.in-addr.arpa", "example.com", 300),
		{
			"id": "srv1", "type": "SRV", "name": "_sip._tcp.example.com",
			"content": "10 20 5060 sip.example.com", "ttl": 300, "proxied": false,
			"data": map[string]any{
				"service": "_sip", "proto": "_tcp", "name": "example.com",
				"priority": 10, "weight": 20, "port": 5060, "target": "sip.example.com",
			},
		},
		{
			"id": "caa1", "type": "CAA", "name": "example.com",
			"content": `0 issue "letsencrypt.org"`, "ttl": 300, "proxied": false,
			"data": map[string]any{"flags": 0, "tag": "issue", "value": "letsencrypt.org"},
		},
	}
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(records))
	}))

	got, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 9)

	// MX — priority
	mx := got[4]
	assert.Equal(t, "MX", mx.Type)
	assert.Equal(t, 10, mx.Priority)

	// SRV — Extra fields populated
	srv := got[7]
	assert.Equal(t, "SRV", srv.Type)
	assert.Equal(t, 10, srv.Priority)
	assert.Equal(t, "_sip", srv.Extra["service"])
	assert.Equal(t, "_tcp", srv.Extra["proto"])
	assert.Equal(t, "20", srv.Extra["weight"])
	assert.Equal(t, "5060", srv.Extra["port"])
	assert.Equal(t, "sip.example.com", srv.Extra["target"])

	// CAA — Extra fields populated
	caa := got[8]
	assert.Equal(t, "CAA", caa.Type)
	assert.Equal(t, "0", caa.Extra["flags"])
	assert.Equal(t, "issue", caa.Extra["tag"])
	assert.Equal(t, "letsencrypt.org", caa.Extra["value"])
}

func TestCloudflare_ListRecords_Empty(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]any{}))
	}))

	records, err := p.ListRecords(context.Background(), "", RecordFilter{})
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestCloudflare_ListRecords_FilterPassedToURL(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "A", r.URL.Query().Get("type"))
		assert.Equal(t, "api.example.com", r.URL.Query().Get("name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]any{}))
	}))

	_, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{Type: "A", Name: "api.example.com"})
	require.NoError(t, err)
}

func TestCloudflare_ListRecords_UsesProviderZoneWhenEmpty(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/zones/"+testZoneID+"/")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess([]any{}))
	}))

	_, err := p.ListRecords(context.Background(), "", RecordFilter{})
	require.NoError(t, err)
}

func TestCloudflare_ListRecords_Unauthorized(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(cfError(10000, "Invalid credentials"))
	}))

	_, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{})
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestCloudflare_ListRecords_RateLimit(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))

	_, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{})
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

func TestCloudflare_ListRecords_APIReturnsFalseSuccess(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfError(1004, "Zone not found"))
	}))

	_, err := p.ListRecords(context.Background(), testZoneID, RecordFilter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Zone not found")
}

// ── CreateRecord ──────────────────────────────────────────────────────────────

func TestCloudflare_CreateRecord_HappyPath(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		var body cloudflareCreateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "A", body.Type)
		assert.Equal(t, "api.example.com", body.Name)
		assert.Equal(t, "5.6.7.8", body.Content)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(cfRecordJSON("new-id", "A", "api.example.com", "5.6.7.8", 300)))
	}))

	rec, err := p.CreateRecord(context.Background(), testZoneID, Record{
		Type: "A", Name: "api.example.com", Content: "5.6.7.8", TTL: 300,
	})
	require.NoError(t, err)
	assert.Equal(t, "new-id", rec.ID)
}

func TestCloudflare_CreateRecord_MX(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body cloudflareCreateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "MX", body.Type)
		assert.Equal(t, 10, body.Priority)

		prio := 10
		result := map[string]any{
			"id": "mx-new", "type": "MX", "name": "example.com",
			"content": "mail.example.com", "ttl": 300, "priority": prio, "proxied": false,
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	rec, err := p.CreateRecord(context.Background(), testZoneID, Record{
		Type: "MX", Name: "example.com", Content: "mail.example.com", TTL: 300, Priority: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, "mx-new", rec.ID)
	assert.Equal(t, 10, rec.Priority)
}

func TestCloudflare_CreateRecord_SRV(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request uses data object, not content
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "SRV", body["type"])
		dataObj, ok := body["data"].(map[string]any)
		require.True(t, ok, "SRV request should have data object")
		assert.Equal(t, "_sip", dataObj["service"])
		assert.Equal(t, "_tcp", dataObj["proto"])
		assert.Equal(t, float64(10), dataObj["priority"])
		assert.Equal(t, float64(20), dataObj["weight"])
		assert.Equal(t, float64(5060), dataObj["port"])
		assert.Equal(t, "sip.example.com", dataObj["target"])

		result := map[string]any{
			"id": "srv-new", "type": "SRV", "name": "_sip._tcp.example.com",
			"content": "10 20 5060 sip.example.com", "ttl": 300, "proxied": false,
			"data": map[string]any{
				"service": "_sip", "proto": "_tcp", "name": "example.com",
				"priority": 10, "weight": 20, "port": 5060, "target": "sip.example.com",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	rec, err := p.CreateRecord(context.Background(), testZoneID, Record{
		Type:     RecordTypeSRV,
		Name:     "example.com",
		TTL:      300,
		Priority: 10,
		Extra: map[string]string{
			"service": "_sip",
			"proto":   "_tcp",
			"weight":  "20",
			"port":    "5060",
			"target":  "sip.example.com",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "srv-new", rec.ID)
	assert.Equal(t, "_sip", rec.Extra["service"])
	assert.Equal(t, "20", rec.Extra["weight"])
	assert.Equal(t, "5060", rec.Extra["port"])
}

func TestCloudflare_CreateRecord_CAA(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "CAA", body["type"])
		dataObj, ok := body["data"].(map[string]any)
		require.True(t, ok, "CAA request should have data object")
		assert.Equal(t, float64(0), dataObj["flags"])
		assert.Equal(t, "issue", dataObj["tag"])
		assert.Equal(t, "letsencrypt.org", dataObj["value"])

		result := map[string]any{
			"id": "caa-new", "type": "CAA", "name": "example.com",
			"content": `0 issue "letsencrypt.org"`, "ttl": 300, "proxied": false,
			"data": map[string]any{"flags": 0, "tag": "issue", "value": "letsencrypt.org"},
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	rec, err := p.CreateRecord(context.Background(), testZoneID, Record{
		Type: RecordTypeCAA,
		Name: "example.com",
		TTL:  300,
		Extra: map[string]string{
			"flags": "0",
			"tag":   "issue",
			"value": "letsencrypt.org",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "caa-new", rec.ID)
	assert.Equal(t, "issue", rec.Extra["tag"])
	assert.Equal(t, "letsencrypt.org", rec.Extra["value"])
}

func TestCloudflare_CreateRecord_Forbidden(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(cfError(10000, "Forbidden"))
	}))

	_, err := p.CreateRecord(context.Background(), testZoneID, Record{Type: "A", Name: "x", Content: "1.2.3.4", TTL: 1})
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// ── UpdateRecord ──────────────────────────────────────────────────────────────

func TestCloudflare_UpdateRecord_HappyPath(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Contains(t, r.URL.Path, "/dns_records/rec-id-1")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(cfRecordJSON("rec-id-1", "A", "example.com", "9.9.9.9", 300)))
	}))

	rec, err := p.UpdateRecord(context.Background(), testZoneID, "rec-id-1", Record{
		Type: "A", Name: "example.com", Content: "9.9.9.9", TTL: 300,
	})
	require.NoError(t, err)
	assert.Equal(t, "rec-id-1", rec.ID)
	assert.Equal(t, "9.9.9.9", rec.Content)
}

func TestCloudflare_UpdateRecord_FullReplacement(t *testing.T) {
	// PUT semantics: verify the full record is sent, not a diff
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		var body cloudflareCreateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "updated.example.com", body.Name)
		assert.Equal(t, "8.8.8.8", body.Content)
		assert.Equal(t, 600, body.TTL)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(cfRecordJSON("id-1", "A", "updated.example.com", "8.8.8.8", 600)))
	}))

	_, err := p.UpdateRecord(context.Background(), testZoneID, "id-1", Record{
		Type: "A", Name: "updated.example.com", Content: "8.8.8.8", TTL: 600,
	})
	require.NoError(t, err)
}

func TestCloudflare_UpdateRecord_NotFound(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(cfError(1032, "Record not found"))
	}))

	_, err := p.UpdateRecord(context.Background(), testZoneID, "ghost-id", Record{Type: "A", Name: "x", Content: "1.1.1.1"})
	assert.ErrorIs(t, err, ErrRecordNotFound)
}

// ── DeleteRecord ──────────────────────────────────────────────────────────────

func TestCloudflare_DeleteRecord_HappyPath(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Contains(t, r.URL.Path, "/dns_records/del-id")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(map[string]any{"id": "del-id"}))
	}))

	err := p.DeleteRecord(context.Background(), testZoneID, "del-id")
	assert.NoError(t, err)
}

func TestCloudflare_DeleteRecord_NotFound(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(cfError(1032, "Record not found"))
	}))

	err := p.DeleteRecord(context.Background(), testZoneID, "ghost")
	assert.ErrorIs(t, err, ErrRecordNotFound)
}

// ── GetNameservers ────────────────────────────────────────────────────────────

func TestCloudflare_GetNameservers_HappyPath(t *testing.T) {
	ns := []string{"ns1.cloudflare.com", "ns2.cloudflare.com"}
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/zones/"+testZoneID, r.URL.Path)

		result := map[string]any{"name_servers": ns}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	got, err := p.GetNameservers(context.Background(), testZoneID)
	require.NoError(t, err)
	assert.Equal(t, ns, got)
}

func TestCloudflare_GetNameservers_FallbackToProviderZone(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/zones/"+testZoneID, r.URL.Path)
		result := map[string]any{"name_servers": []string{"ns1.cf.com"}}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	_, err := p.GetNameservers(context.Background(), "")
	require.NoError(t, err)
}

func TestCloudflare_GetNameservers_ZoneNotFound(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(cfError(1001, "Zone not found"))
	}))

	_, err := p.GetNameservers(context.Background(), testZoneID)
	assert.ErrorIs(t, err, ErrRecordNotFound)
}

func TestCloudflare_GetNameservers_EmptyList(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := map[string]any{"name_servers": []string{}}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(result))
	}))

	_, err := p.GetNameservers(context.Background(), testZoneID)
	assert.ErrorIs(t, err, ErrZoneNotFound)
}

// ── BatchCreateRecords ────────────────────────────────────────────────────────

func TestCloudflare_BatchCreateRecords_HappyPath(t *testing.T) {
	call := 0
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		id := fmt.Sprintf("created-%d", call)
		var body cloudflareCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(cfRecordJSON(id, body.Type, body.Name, body.Content, body.TTL)))
	}))

	in := []Record{
		{Type: "A", Name: "a.example.com", Content: "1.1.1.1", TTL: 300},
		{Type: "A", Name: "b.example.com", Content: "2.2.2.2", TTL: 300},
		{Type: "CNAME", Name: "c.example.com", Content: "a.example.com", TTL: 1},
	}
	out, err := p.BatchCreateRecords(context.Background(), testZoneID, in)
	require.NoError(t, err)
	assert.Len(t, out, 3)
	assert.Equal(t, "created-1", out[0].ID)
	assert.Equal(t, "created-2", out[1].ID)
	assert.Equal(t, "created-3", out[2].ID)
}

func TestCloudflare_BatchCreateRecords_PartialFailure(t *testing.T) {
	call := 0
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		var body cloudflareCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(cfRecordJSON(fmt.Sprintf("id-%d", call), body.Type, body.Name, body.Content, body.TTL)))
	}))

	in := []Record{
		{Type: "A", Name: "ok.example.com", Content: "1.1.1.1", TTL: 300},
		{Type: "A", Name: "fail.example.com", Content: "2.2.2.2", TTL: 300},
		{Type: "A", Name: "never.example.com", Content: "3.3.3.3", TTL: 300},
	}
	out, err := p.BatchCreateRecords(context.Background(), testZoneID, in)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
	assert.Len(t, out, 1)
	assert.Equal(t, "id-1", out[0].ID)
}

func TestCloudflare_BatchCreateRecords_EmptySlice(t *testing.T) {
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call API for empty slice")
	}))

	out, err := p.BatchCreateRecords(context.Background(), testZoneID, []Record{})
	require.NoError(t, err)
	assert.Empty(t, out)
}

// ── BatchDeleteRecords ────────────────────────────────────────────────────────

func TestCloudflare_BatchDeleteRecords_HappyPath(t *testing.T) {
	deleted := []string{}
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		parts := splitPath(r.URL.Path)
		deleted = append(deleted, parts[len(parts)-1])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(map[string]any{"id": parts[len(parts)-1]}))
	}))

	err := p.BatchDeleteRecords(context.Background(), testZoneID, []string{"id-1", "id-2", "id-3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"id-1", "id-2", "id-3"}, deleted)
}

func TestCloudflare_BatchDeleteRecords_StopsOnFirstError(t *testing.T) {
	call := 0
	p, _ := cfProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 2 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(cfError(1032, "record not found"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(cfSuccess(map[string]any{"id": "x"}))
	}))

	err := p.BatchDeleteRecords(context.Background(), testZoneID, []string{"ok-1", "missing", "never"})
	assert.ErrorIs(t, err, ErrRecordNotFound)
	assert.Equal(t, 2, call)
}

// ── cfCheckStatus ─────────────────────────────────────────────────────────────

func TestCfCheckStatus(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		body    []byte
		wantErr error
		wantMsg string
	}{
		{"200 ok", http.StatusOK, nil, nil, ""},
		{"201 created", http.StatusCreated, nil, nil, ""},
		{"204 no content", http.StatusNoContent, nil, nil, ""},
		{"401 unauthorized", http.StatusUnauthorized, cfError(10000, "bad token"), ErrUnauthorized, ""},
		{"403 forbidden", http.StatusForbidden, cfError(10000, "forbidden"), ErrUnauthorized, ""},
		{"404 not found", http.StatusNotFound, cfError(1032, "record not found"), ErrRecordNotFound, ""},
		{"429 rate limit", http.StatusTooManyRequests, []byte(`{}`), ErrRateLimitExceeded, ""},
		{"500 with cf error", http.StatusInternalServerError, cfError(1000, "server error"), nil, "cloudflare error 1000: server error"},
		{"500 raw body", http.StatusInternalServerError, []byte("internal server error"), nil, "cloudflare HTTP 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cfCheckStatus(tt.code, tt.body)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else if tt.wantMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCfCheckStatus_LongBodyTruncated(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	err := cfCheckStatus(http.StatusInternalServerError, long)
	require.Error(t, err)
	assert.LessOrEqual(t, len(err.Error()), 300)
}

// ── cfBuildRequest ────────────────────────────────────────────────────────────

func TestCfBuildRequest_StandardRecord(t *testing.T) {
	data, err := cfBuildRequest(Record{Type: "A", Name: "x.example.com", Content: "1.2.3.4", TTL: 300})
	require.NoError(t, err)
	var body cloudflareCreateRequest
	require.NoError(t, json.Unmarshal(data, &body))
	assert.Equal(t, "A", body.Type)
	assert.Equal(t, "1.2.3.4", body.Content)
}

func TestCfBuildRequest_SRV_UsesDataObject(t *testing.T) {
	data, err := cfBuildRequest(Record{
		Type:     RecordTypeSRV,
		Name:     "example.com",
		TTL:      300,
		Priority: 10,
		Extra:    map[string]string{"service": "_sip", "proto": "_tcp", "weight": "20", "port": "5060", "target": "sip.example.com"},
	})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	assert.Equal(t, "SRV", body["type"])
	assert.Nil(t, body["content"], "SRV should not have content field")
	_, hasData := body["data"]
	assert.True(t, hasData, "SRV should have data field")
}

func TestCfBuildRequest_CAA_UsesDataObject(t *testing.T) {
	data, err := cfBuildRequest(Record{
		Type:  RecordTypeCAA,
		Name:  "example.com",
		TTL:   300,
		Extra: map[string]string{"flags": "0", "tag": "issue", "value": "letsencrypt.org"},
	})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(data, &body))
	assert.Equal(t, "CAA", body["type"])
	_, hasData := body["data"]
	assert.True(t, hasData)
}

// ── Name() ────────────────────────────────────────────────────────────────────

func TestCloudflare_Name(t *testing.T) {
	p, _ := cfProvider(t, http.NewServeMux())
	assert.Equal(t, "cloudflare", p.Name())
}

// ── helpers ───────────────────────────────────────────────────────────────────

// splitPath splits a URL path into non-empty parts.
func splitPath(path string) []string {
	var parts []string
	cur := ""
	for _, c := range path {
		if c == '/' {
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return parts
}
