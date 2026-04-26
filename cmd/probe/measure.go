package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"domain-platform/cmd/probe/checker"
	"domain-platform/pkg/probeprotocol"
)

// measureLoop periodically fetches assignments and runs the 4-layer check for each.
func measureLoop(ctx context.Context, client *http.Client, baseURL, nodeID, nodeRole, dnsResolver string, intervalSecs int, logger *zap.Logger) {
	if intervalSecs <= 0 {
		intervalSecs = 180
	}

	c := checker.New(dnsResolver, checker.BogonList{}, probeVersion, logger)

	// Run immediately on startup, then on interval.
	runMeasurements(ctx, c, client, baseURL, nodeID, nodeRole, logger)

	ticker := time.NewTicker(time.Duration(intervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runMeasurements(ctx, c, client, baseURL, nodeID, nodeRole, logger)
		}
	}
}

func runMeasurements(ctx context.Context, c *checker.Checker, client *http.Client, baseURL, nodeID, nodeRole string, logger *zap.Logger) {
	assignments, err := fetchAssignments(ctx, client, baseURL, nodeID)
	if err != nil {
		logger.Warn("fetch assignments failed", zap.String("node_id", nodeID), zap.Error(err))
		return
	}

	if len(assignments) == 0 {
		logger.Debug("no assignments for this node", zap.String("node_id", nodeID))
		return
	}

	logger.Info("starting measurement batch",
		zap.String("node_id", nodeID),
		zap.Int("count", len(assignments)),
	)

	results := make([]probeprotocol.Measurement, 0, len(assignments))
	for _, a := range assignments {
		m := c.FullCheck(ctx, a, nodeID, nodeRole)
		results = append(results, m)
	}

	if err := submitMeasurements(ctx, client, baseURL, nodeID, results, logger); err != nil {
		logger.Warn("submit measurements failed", zap.String("node_id", nodeID), zap.Error(err))
	}
}

// fetchAssignments GETs the domain assignments for this probe node.
func fetchAssignments(ctx context.Context, client *http.Client, baseURL, nodeID string) ([]probeprotocol.Assignment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/probe/v1/assignments?node_id="+nodeID, nil)
	if err != nil {
		return nil, fmt.Errorf("build assignments request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("assignments GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("assignments: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Data probeprotocol.AssignmentsResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode assignments response: %w", err)
	}

	return result.Data.Assignments, nil
}

// submitMeasurements POSTs measurement results to the control plane.
func submitMeasurements(ctx context.Context, client *http.Client, baseURL, nodeID string, measurements []probeprotocol.Measurement, logger *zap.Logger) error {
	payload := probeprotocol.SubmitMeasurementsRequest{
		NodeID:       nodeID,
		Measurements: measurements,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal measurements: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/probe/v1/measurements", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build measurements request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("measurements POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("submit measurements: unexpected status %d", resp.StatusCode)
	}

	logger.Info("measurements submitted",
		zap.String("node_id", nodeID),
		zap.Int("count", len(measurements)),
	)
	return nil
}
