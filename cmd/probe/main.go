// cmd/probe is the GFW probe node binary.
//
// A probe node is a pull-based measurement agent deployed inside or near the
// Great Firewall. It registers with the control plane, then runs two concurrent
// loops:
//   - Heartbeat loop: periodic alive ping (POST /probe/v1/heartbeat)
//   - Measure loop:   fetch assignments → run 4-layer check → submit results
//
// Safety boundary (see safety.go):
//   - NO os/exec — all network operations use pure Go stdlib + miekg/dns
//   - NO provider credentials — probe nodes hold only their mTLS cert
//   - Whitelist-only: register, heartbeat, get-assignments, submit-measurements
//
// CI gate: `make check-probe-safety` enforces the no-os/exec rule via grep.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"domain-platform/internal/bootstrap"
	"domain-platform/pkg/probeprotocol"
)

const probeVersion = "0.1.0"

func main() {
	cfg, err := bootstrap.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := bootstrap.NewLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	probeCfg := cfg.Probe
	if probeCfg.ControlPlaneURL == "" {
		logger.Fatal("probe.control_plane_url is required")
	}
	if probeCfg.NodeID == "" {
		// Fall back to hostname if not explicitly configured
		probeCfg.NodeID = getHostname()
		logger.Warn("probe.node_id not set; using hostname", zap.String("node_id", probeCfg.NodeID))
	}

	httpClient, err := buildProbeHTTPClient(probeCfg)
	if err != nil {
		logger.Fatal("build HTTP client", zap.Error(err))
	}

	logger.Info("probe starting",
		zap.String("version", probeVersion),
		zap.String("node_id", probeCfg.NodeID),
		zap.String("region", probeCfg.Region),
		zap.String("role", probeCfg.Role),
		zap.String("control_plane", probeCfg.ControlPlaneURL),
	)

	// ── Register with control plane ──────────────────────────────────────
	regReq := probeprotocol.RegisterRequest{
		NodeID:       probeCfg.NodeID,
		Region:       probeCfg.Region,
		Role:         probeCfg.Role,
		ProbeVersion: probeVersion,
	}

	regResp, err := register(httpClient, probeCfg.ControlPlaneURL, regReq, logger)
	if err != nil {
		logger.Fatal("register with control plane", zap.Error(err))
	}

	// Use intervals from control plane response if non-zero; fall back to config.
	heartbeatSecs := regResp.HeartbeatSecs
	if heartbeatSecs <= 0 {
		heartbeatSecs = probeCfg.HeartbeatSecs
	}
	checkInterval := probeCfg.CheckInterval
	if checkInterval <= 0 {
		checkInterval = 180
	}

	// ── Start loops ──────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go heartbeatLoop(ctx, httpClient, probeCfg.ControlPlaneURL, probeCfg.NodeID, heartbeatSecs, logger)
	go measureLoop(ctx, httpClient, probeCfg.ControlPlaneURL, probeCfg.NodeID, probeCfg.Role, probeCfg.DNSResolver, checkInterval, logger)

	// ── Wait for shutdown signal ─────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info("shutting down", zap.String("signal", sig.String()))
	cancel()

	// Allow in-flight measurements to drain
	time.Sleep(3 * time.Second)
	logger.Info("probe exited cleanly")
}

// buildProbeHTTPClient returns an *http.Client configured for mTLS when cert
// files are present, or plain HTTP for development.
func buildProbeHTTPClient(cfg bootstrap.ProbeConfig) (*http.Client, error) {
	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load probe cert: %w", err)
		}

		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		}

		if cfg.CACertFile != "" {
			caCert, err := os.ReadFile(cfg.CACertFile)
			if err != nil {
				return nil, fmt.Errorf("read CA cert: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("parse CA cert")
			}
			tlsCfg.RootCAs = pool
		}

		transport.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}, nil
}
