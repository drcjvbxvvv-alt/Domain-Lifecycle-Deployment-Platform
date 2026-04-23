package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyTLSError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"connection reset", "read: connection reset by peer", "connection_reset"},
		{"connection_reset underscore", "connection_reset", "connection_reset"},
		{"timeout", "dial timeout exceeded", "timeout"},
		{"deadline", "context deadline exceeded", "timeout"},
		{"cert error", "x509 certificate signed by unknown authority", "cert_error"},
		{"cert short", "cert validation failed", "cert_error"},
		{"unknown", "some other error", "some other error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyTLSError(wrapErr(tt.input))
			assert.Equal(t, tt.want, err)
		})
	}
}

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"connection reset", "connection reset by peer", "connection_reset"},
		{"timeout", "connection timeout exceeded", "timeout"},
		{"deadline exceeded", "context deadline exceeded", "timeout"},
		{"tls error", "tls handshake failed", "tls_error"},
		{"certificate", "certificate verify failed", "tls_error"},
		{"unknown", "EOF", "EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHTTPError(wrapErr(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsAny(t *testing.T) {
	assert.True(t, containsAny("connection reset by peer", "connection reset"))
	assert.True(t, containsAny("timeout exceeded", "timeout", "deadline"))
	assert.False(t, containsAny("normal error", "timeout", "reset"))
	assert.False(t, containsAny("", "timeout"))
	assert.True(t, containsAny("x509 certificate error", "certificate", "cert"))
}

// wrapErr converts a string into an error for classifyXxx calls.
type errString string

func (e errString) Error() string { return string(e) }

func wrapErr(s string) error { return errString(s) }
