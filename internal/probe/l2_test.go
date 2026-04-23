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

func TestL2Checker_Tier(t *testing.T) {
	assert.Equal(t, int16(2), NewL2Checker().Tier())
}

func TestL2Checker_OK_NoAssertions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>hello world</body></html>")
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusOK, result.Status)
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusOK, *result.HTTPStatus)
	require.NotNil(t, result.ContentHash)
	assert.NotEmpty(t, *result.ContentHash)
	assert.Nil(t, result.ErrorMessage)
}

func TestL2Checker_KeywordFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>Welcome to example.com — powered by platform v2</body></html>")
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	kw := "platform v2"
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		ExpectedKeyword: &kw,
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusOK, result.Status)
	detail, ok := result.Detail.(*L2Detail)
	require.True(t, ok)
	assert.True(t, detail.KeywordFound)
	assert.Equal(t, kw, detail.Keyword)
}

func TestL2Checker_KeywordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>hello</body></html>")
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	kw := "MISSING_KEYWORD_XYZ"
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		ExpectedKeyword: &kw,
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusFail, result.Status)
	require.NotNil(t, result.ErrorMessage)
	assert.Contains(t, *result.ErrorMessage, "keyword")

	detail, ok := result.Detail.(*L2Detail)
	require.True(t, ok)
	assert.False(t, detail.KeywordFound)
}

func TestL2Checker_MetaVersionMatch(t *testing.T) {
	const version = "v20260408-001"
	body := fmt.Sprintf(`<html><head>
		<meta name="release-version" content="%s">
		</head><body>ok</body></html>`, version)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		ExpectedMetaTag: strPtr(version),
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusOK, result.Status)
	detail, ok := result.Detail.(*L2Detail)
	require.True(t, ok)
	assert.Equal(t, version, detail.MetaVersionDetected)
	assert.True(t, detail.MetaVersionMatch)
}

func TestL2Checker_MetaVersionMismatch(t *testing.T) {
	body := `<html><head>
		<meta name="release-version" content="v20260408-001">
		</head><body>ok</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		ExpectedMetaTag: strPtr("v20260409-999"), // different version
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusFail, result.Status)
	detail, ok := result.Detail.(*L2Detail)
	require.True(t, ok)
	assert.False(t, detail.MetaVersionMatch)
	assert.Equal(t, "v20260408-001", detail.MetaVersionDetected)
}

func TestL2Checker_MetaVersionNotPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body>no meta here</body></html>")
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:            host,
		ExpectedMetaTag: strPtr("v20260408-001"),
		TimeoutSeconds:  5,
	})

	assert.Equal(t, StatusFail, result.Status)
	detail, ok := result.Detail.(*L2Detail)
	require.True(t, ok)
	assert.Empty(t, detail.MetaVersionDetected)
	assert.False(t, detail.MetaVersionMatch)
}

func TestL2Checker_StatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	expectedStatus := 200
	checker := NewL2Checker()
	result := checker.Check(context.Background(), CheckRequest{
		FQDN:           host,
		ExpectedStatus: &expectedStatus,
		TimeoutSeconds: 5,
	})

	assert.Equal(t, StatusFail, result.Status)
	require.NotNil(t, result.ErrorMessage)
	assert.Contains(t, *result.ErrorMessage, "404")
}

func TestL2Checker_ContentHash_Deterministic(t *testing.T) {
	const body = "<html><body>fixed content</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	checker := NewL2Checker()

	r1 := checker.Check(context.Background(), CheckRequest{FQDN: host, TimeoutSeconds: 5})
	r2 := checker.Check(context.Background(), CheckRequest{FQDN: host, TimeoutSeconds: 5})

	require.NotNil(t, r1.ContentHash)
	require.NotNil(t, r2.ContentHash)
	assert.Equal(t, *r1.ContentHash, *r2.ContentHash)
}

// extractMetaVersion is a test helper that mirrors the logic in l2.go.
func extractMetaVersion(body string) string {
	for _, tag := range metaTagRe.FindAllString(body, -1) {
		if metaNameRe.MatchString(tag) {
			if m := metaContentRe.FindStringSubmatch(tag); len(m) > 1 {
				return m[1]
			}
		}
	}
	return ""
}

func TestMetaVersionRe_Variants(t *testing.T) {
	cases := []struct {
		html     string
		expected string
	}{
		{`<meta name="release-version" content="v1">`, "v1"},
		{`<meta name='release-version' content='v2'>`, "v2"},
		{`<META NAME="RELEASE-VERSION" CONTENT="v3">`, "v3"},
		{`<meta content="v4" name="release-version">`, "v4"}, // content before name
		{`<meta name="other" content="nope">`, ""},
		{`no meta at all`, ""},
	}

	for _, tc := range cases {
		got := extractMetaVersion(tc.html)
		assert.Equal(t, tc.expected, got, "html: %q", tc.html)
	}
}
