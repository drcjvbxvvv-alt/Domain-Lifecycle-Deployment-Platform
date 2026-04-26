package notify

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailSender_MissingHost(t *testing.T) {
	cfg, _ := json.Marshal(EmailConfig{
		FromAddress: "from@example.com",
		ToAddresses: []string{"to@example.com"},
	})
	sender := NewEmailSender()
	err := sender.Send(context.Background(), cfg, Message{Subject: "test", Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp_host")
}

func TestEmailSender_MissingFrom(t *testing.T) {
	cfg, _ := json.Marshal(EmailConfig{
		SMTPHost:    "smtp.example.com",
		ToAddresses: []string{"to@example.com"},
	})
	sender := NewEmailSender()
	err := sender.Send(context.Background(), cfg, Message{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "from_address")
}

func TestEmailSender_MissingToAddresses(t *testing.T) {
	cfg, _ := json.Marshal(EmailConfig{
		SMTPHost:    "smtp.example.com",
		FromAddress: "from@example.com",
	})
	sender := NewEmailSender()
	err := sender.Send(context.Background(), cfg, Message{Body: "body"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to_address")
}

func TestEmailSender_InvalidConfig(t *testing.T) {
	sender := NewEmailSender()
	err := sender.Send(context.Background(), json.RawMessage(`not-json`), Message{Body: "test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config")
}

func TestEmailSender_DefaultPort(t *testing.T) {
	// Verify that SMTPPort defaults to 587 when 0 is given.
	cfg, err := parseEmailConfig(mustMarshal(EmailConfig{
		SMTPHost:    "smtp.example.com",
		FromAddress: "from@example.com",
		ToAddresses: []string{"to@example.com"},
	}))
	require.NoError(t, err)
	assert.Equal(t, 587, cfg.SMTPPort)
}

func TestEmailSender_ExplicitPort(t *testing.T) {
	cfg, err := parseEmailConfig(mustMarshal(EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    465,
		FromAddress: "from@example.com",
		ToAddresses: []string{"to@example.com"},
		UseTLS:      true,
	}))
	require.NoError(t, err)
	assert.Equal(t, 465, cfg.SMTPPort)
	assert.True(t, cfg.UseTLS)
}

// mustMarshal is a test helper.
func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
