package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"domain-platform/pkg/probeprotocol"
)

// heartbeatLoop sends periodic heartbeats to the control plane.
// Runs until the context is cancelled. Implements exponential backoff on failure.
func heartbeatLoop(ctx context.Context, client *http.Client, baseURL, nodeID string, intervalSecs int, logger *zap.Logger) {
	if intervalSecs <= 0 {
		intervalSecs = 30
	}
	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	failCount := 0
	const maxBackoffSecs = 90

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := sendHeartbeat(ctx, client, baseURL, nodeID, logger)
			if err != nil {
				failCount++
				backoff := intervalSecs * failCount
				if backoff > maxBackoffSecs {
					backoff = maxBackoffSecs
				}
				logger.Warn("heartbeat failed",
					zap.String("node_id", nodeID),
					zap.Error(err),
					zap.Int("fail_count", failCount),
					zap.Int("backoff_secs", backoff),
				)
				ticker.Reset(time.Duration(backoff) * time.Second)
			} else {
				if failCount > 0 {
					logger.Info("heartbeat recovered", zap.Int("prev_failures", failCount))
					ticker.Reset(time.Duration(intervalSecs) * time.Second)
				}
				failCount = 0
			}
		}
	}
}

func sendHeartbeat(ctx context.Context, client *http.Client, baseURL, nodeID string, logger *zap.Logger) error {
	req := probeprotocol.HeartbeatRequest{
		NodeID:       nodeID,
		ProbeVersion: probeVersion,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/probe/v1/heartbeat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create heartbeat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("heartbeat POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Data probeprotocol.HeartbeatResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode heartbeat response: %w", err)
	}

	if result.Data.HasNewDomains {
		logger.Debug("control plane signals new domain assignments available")
	}

	return nil
}
