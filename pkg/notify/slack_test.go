package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlackSender_Send_Success(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(SlackConfig{WebhookURL: srv.URL})
	sender := NewSlackSender()

	err := sender.Send(context.Background(), cfg, Message{
		Subject:  "Alert",
		Body:     "Something failed",
		Severity: "critical",
	})
	require.NoError(t, err)
	assert.Contains(t, received["text"], "Alert")
	assert.Contains(t, received["text"], "Something failed")
}

func TestSlackSender_Send_WithUsernameAndChannel(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(SlackConfig{
		WebhookURL: srv.URL,
		Username:   "DomainBot",
		Channel:    "#alerts",
	})
	sender := NewSlackSender()

	err := sender.Send(context.Background(), cfg, Message{Subject: "Test", Body: "body"})
	require.NoError(t, err)
	assert.Equal(t, "DomainBot", received["username"])
	assert.Equal(t, "#alerts", received["channel"])
}

func TestSlackSender_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(SlackConfig{WebhookURL: srv.URL})
	sender := NewSlackSender()

	err := sender.Send(context.Background(), cfg, Message{Body: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
}

func TestSlackSender_Send_MissingWebhookURL(t *testing.T) {
	cfg, _ := json.Marshal(SlackConfig{})
	sender := NewSlackSender()

	err := sender.Send(context.Background(), cfg, Message{Body: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook_url")
}

func TestSlackSender_Send_InvalidConfig(t *testing.T) {
	sender := NewSlackSender()
	err := sender.Send(context.Background(), json.RawMessage(`not-json`), Message{Body: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config")
}

func TestSlackSender_Test(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(SlackConfig{WebhookURL: srv.URL})
	sender := NewSlackSender()

	err := sender.Test(context.Background(), cfg)
	require.NoError(t, err)
}
