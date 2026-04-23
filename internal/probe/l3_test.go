package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestL3Checker_Tier(t *testing.T) {
	assert.Equal(t, int16(3), NewL3Checker().Tier())
}

func TestL3Checker_HealthOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status":"ok"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL3Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		HealthPath:     "/health",
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusOK, result.Status)
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusOK, *result.HTTPStatus)

	detail, ok := result.Detail.(*L3Detail)
	require.True(t, ok)
	assert.Contains(t, detail.HealthURL, "/health")
	assert.Contains(t, detail.ResponseSnippet, "ok")
}

func TestL3Checker_HealthFail_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"status":"degraded"}`)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL3Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		HealthPath:     "/health",
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusFail, result.Status)
	require.NotNil(t, result.ErrorMessage)
	assert.Contains(t, *result.ErrorMessage, "503")
}

func TestL3Checker_KeywordFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok","version":"v2"}`)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	kw := `"status":"ok"`
	checker := NewL3Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		HealthPath:      "/health",
		ExpectedKeyword: &kw,
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusOK, result.Status)
	detail, ok := result.Detail.(*L3Detail)
	require.True(t, ok)
	assert.True(t, detail.KeywordFound)
}

func TestL3Checker_KeywordMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"degraded"}`)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	kw := `"status":"ok"`
	checker := NewL3Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		HealthPath:      "/health",
		ExpectedKeyword: &kw,
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusFail, result.Status)
	detail, ok := result.Detail.(*L3Detail)
	require.True(t, ok)
	assert.False(t, detail.KeywordFound)
}

func TestL3Checker_DefaultHealthPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only /health returns 200; everything else returns 404.
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL3Checker()
	// HealthPath omitted → should default to /health.
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusOK, result.Status)
	detail, ok := result.Detail.(*L3Detail)
	require.True(t, ok)
	assert.Contains(t, detail.HealthURL, "/health")
}

func TestL3Checker_HealthPathWithoutLeadingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL3Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		HealthPath:     "api/health", // no leading slash — checker should add it
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusOK, result.Status)
}
