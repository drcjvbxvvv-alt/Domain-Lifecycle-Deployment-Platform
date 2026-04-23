// Safety boundary for cmd/probe (GFW probe node binary).
//
// The probe binary MUST NOT:
//   - Execute arbitrary shell commands (os/exec is NOT imported here)
//   - Pull from arbitrary URLs beyond the configured control-plane base URL
//   - Write to arbitrary file paths
//   - Hold provider credentials (DNS tokens, CDN keys, etc.)
//   - Operate on any host other than itself
//
// This file is intentionally free of os/exec imports.
// The CI gate `make check-probe-safety` enforces the same rule via grep:
//
//	grep -r 'os/exec' cmd/probe/ && exit 1 || exit 0
//
// Any PR adding an os/exec call to cmd/probe/ is REJECTED without Opus review.
//
// The probe node's allowed actions (whitelist):
//  1. Register with the control plane (POST /probe/v1/register)
//  2. Send periodic heartbeats (POST /probe/v1/heartbeat)
//  3. Fetch assigned domains (GET /probe/v1/assignments)
//  4. Perform 4-layer network measurements (DNS → TCP → TLS → HTTP) using
//     pure Go network libraries — no shell commands.
//  5. Submit measurement results (POST /probe/v1/measurements)
//
// Measurement implementation constraints:
//   - DNS: use github.com/miekg/dns — raw DNS protocol, no shell resolver
//   - TCP: use net.DialTimeout — pure Go TCP dialer
//   - TLS: use crypto/tls — pure Go TLS stack
//   - HTTP: use net/http — pure Go HTTP client
package main
