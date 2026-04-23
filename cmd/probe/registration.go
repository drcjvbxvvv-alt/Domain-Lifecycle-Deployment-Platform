package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
)

// register performs initial registration with the control plane.
// POST /probe/v1/register
// Idempotent — safe to call on every start-up.
func register(client *http.Client, baseURL string, req probeprotocol.RegisterRequest, logger *zap.Logger) (*probeprotocol.RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal register request: %w", err)
	}

	resp, err := client.Post(baseURL+"/probe/v1/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("register POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("register: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Code    int                              `json:"code"`
		Data    probeprotocol.RegisterResponse   `json:"data"`
		Message string                           `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}

	logger.Info("registered with control plane",
		zap.String("node_id", result.Data.NodeID),
		zap.String("status", result.Data.Status),
		zap.Int("heartbeat_secs", result.Data.HeartbeatSecs),
		zap.Int("check_interval", result.Data.CheckInterval),
	)

	return &result.Data, nil
}

// getHostname returns the system hostname, with a fallback.
func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
