package registrar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ────────────────────────────────────────────────────────────────────

func testProvider(t *testing.T, handler http.Handler) Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := GoDaddyCredentials{
		Key:         "test-key",
		Secret:      "test-secret",
		Environment: "production",
	}
	return newGoDaddyProviderWithClient(creds, srv.URL, srv.Client())
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ── newGoDaddyProvider (factory) ───────────────────────────────────────────────

func TestNewGoDaddyProvider_OK(t *testing.T) {
	creds, _ := json.Marshal(GoDaddyCredentials{Key: "k", Secret: "s"})
	p, err := newGoDaddyProvider(creds)
	require.NoError(t, err)
	assert.Equal(t, "godaddy", p.Name())
}

func TestNewGoDaddyProvider_MissingKey(t *testing.T) {
	creds, _ := json.Marshal(GoDaddyCredentials{Secret: "s"})
	_, err := newGoDaddyProvider(creds)
	assert.ErrorIs(t, err, ErrMissingCredentials)
}

func TestNewGoDaddyProvider_MissingSecret(t *testing.T) {
	creds, _ := json.Marshal(GoDaddyCredentials{Key: "k"})
	_, err := newGoDaddyProvider(creds)
	assert.ErrorIs(t, err, ErrMissingCredentials)
}

func TestNewGoDaddyProvider_InvalidJSON(t *testing.T) {
	_, err := newGoDaddyProvider(json.RawMessage(`{bad json}`))
	assert.ErrorIs(t, err, ErrMissingCredentials)
}

func TestNewGoDaddyProvider_OTEBaseURL(t *testing.T) {
	creds, _ := json.Marshal(GoDaddyCredentials{Key: "k", Secret: "s", Environment: "ote"})
	p, err := newGoDaddyProvider(creds)
	require.NoError(t, err)
	assert.Equal(t, "godaddy", p.Name())
}

// ── ListDomains ────────────────────────────────────────────────────────────────

func TestListDomains_SinglePage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	exp := now.AddDate(1, 0, 0)

	domains := []goDaddyDomainItem{
		{Domain: "example.com", CreatedAt: now.Format(time.RFC3339), Expires: exp.Format(time.RFC3339), RenewAuto: true, Status: "ACTIVE"},
		{Domain: "test.org", CreatedAt: now.Format(time.RFC3339), Expires: exp.Format(time.RFC3339), RenewAuto: false, Status: "ACTIVE"},
	}

	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/domains", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "sso-key test-key:test-secret")
		w.Header().Set("Content-Type", "application/json")
		w.Write(mustJSON(domains))
	}))

	result, err := p.ListDomains(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "example.com", result[0].FQDN)
	assert.True(t, result[0].AutoRenew)
	assert.Equal(t, "ACTIVE", result[0].Status)
	require.NotNil(t, result[0].RegistrationDate)
	require.NotNil(t, result[0].ExpiryDate)
	assert.Equal(t, now.Unix(), result[0].RegistrationDate.Unix())
	assert.Equal(t, exp.Unix(), result[0].ExpiryDate.Unix())

	assert.Equal(t, "test.org", result[1].FQDN)
	assert.False(t, result[1].AutoRenew)
}

func TestListDomains_Pagination(t *testing.T) {
	// Page 1 returns exactly 500 items; page 2 returns 1 item → 501 total.
	calls := 0
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")

		if calls == 1 {
			assert.Empty(t, r.URL.Query().Get("marker"))
			items := make([]goDaddyDomainItem, 500)
			for i := range items {
				items[i] = goDaddyDomainItem{Domain: fmt.Sprintf("domain%04d.com", i), Status: "ACTIVE"}
			}
			w.Write(mustJSON(items))
		} else {
			assert.Equal(t, "domain0499.com", r.URL.Query().Get("marker"))
			items := []goDaddyDomainItem{{Domain: "last.com", Status: "ACTIVE"}}
			w.Write(mustJSON(items))
		}
	}))

	result, err := p.ListDomains(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 501)
	assert.Equal(t, 2, calls)
}

func TestListDomains_EmptyAccount(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))

	result, err := p.ListDomains(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestListDomains_Unauthorized(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Authentication failed"}`))
	}))

	_, err := p.ListDomains(context.Background())
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestListDomains_RateLimit(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	_, err := p.ListDomains(context.Background())
	assert.ErrorIs(t, err, ErrRateLimitExceeded)
}

func TestListDomains_ServerError(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
	}))

	_, err := p.ListDomains(context.Background())
	assert.ErrorContains(t, err, "500")
}

func TestListDomains_FQDNLowercased(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mustJSON([]goDaddyDomainItem{{Domain: "EXAMPLE.COM", Status: "ACTIVE"}}))
	}))

	result, err := p.ListDomains(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "example.com", result[0].FQDN)
}

// ── GetDomain ──────────────────────────────────────────────────────────────────

func TestGetDomain_OK(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	exp := now.AddDate(1, 0, 0)

	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/domains/example.com", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(mustJSON(goDaddyDomainItem{
			Domain:      "example.com",
			CreatedAt:   now.Format(time.RFC3339),
			Expires:     exp.Format(time.RFC3339),
			RenewAuto:   true,
			Status:      "ACTIVE",
			NameServers: []string{"ns1.example.com", "ns2.example.com"},
		}))
	}))

	info, err := p.GetDomain(context.Background(), "example.com")
	require.NoError(t, err)
	assert.Equal(t, "example.com", info.FQDN)
	assert.True(t, info.AutoRenew)
	assert.Equal(t, []string{"ns1.example.com", "ns2.example.com"}, info.NameServers)
}

func TestGetDomain_NotFound(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Domain not found"}`))
	}))

	_, err := p.GetDomain(context.Background(), "notexist.com")
	assert.ErrorIs(t, err, ErrDomainNotFound)
}

func TestGetDomain_Unauthorized(t *testing.T) {
	p := testProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := p.GetDomain(context.Background(), "example.com")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

// ── Registry ───────────────────────────────────────────────────────────────────

func TestRegistryGet_GoDaddy(t *testing.T) {
	creds, _ := json.Marshal(GoDaddyCredentials{Key: "k", Secret: "s"})
	p, err := Get("godaddy", creds)
	require.NoError(t, err)
	assert.Equal(t, "godaddy", p.Name())
}

func TestRegistryGet_UnknownType(t *testing.T) {
	_, err := Get("nonexistent", json.RawMessage(`{}`))
	assert.ErrorIs(t, err, ErrProviderNotRegistered)
}

func TestRegisteredTypes_ContainsGoDaddy(t *testing.T) {
	types := RegisteredTypes()
	assert.Contains(t, types, "godaddy")
}

// ── toDomainInfo ───────────────────────────────────────────────────────────────

func TestToDomainInfo_MissingDates(t *testing.T) {
	// When dates are empty strings, RegistrationDate/ExpiryDate should be nil
	d := goDaddyDomainItem{Domain: "example.com", Status: "ACTIVE"}
	info := toDomainInfo(d)
	assert.Nil(t, info.RegistrationDate)
	assert.Nil(t, info.ExpiryDate)
}

func TestToDomainInfo_InvalidDateFormat(t *testing.T) {
	d := goDaddyDomainItem{
		Domain:    "example.com",
		CreatedAt: "not-a-date",
		Expires:   "also-not-a-date",
	}
	info := toDomainInfo(d)
	assert.Nil(t, info.RegistrationDate)
	assert.Nil(t, info.ExpiryDate)
}
