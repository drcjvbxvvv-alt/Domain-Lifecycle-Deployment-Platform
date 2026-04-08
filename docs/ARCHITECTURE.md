# ARCHITECTURE.md — System Architecture Reference

> This document is a detailed reference for the Domain Lifecycle Management Platform architecture.
> Claude Code should read this when working on cross-cutting concerns, provider integrations, or deployment.

---

## 1. System Overview

The platform manages 12,000+ domains across 10 projects. It automates:
- Domain onboarding (DNS + CDN + nginx conf + deployment + verification)
- Continuous reachability monitoring from 6 CN probe nodes
- Automated failover when GFW blocks a domain (<5 min recovery)
- Batch releases with canary/rollback capabilities
- Standby domain pool management with pre-warming

### Core Principle: Prefix Determines Everything

```
Given: main domain = example.com, prefix = "ws"
System auto-derives:
  FQDN:          ws.example.com
  DNS Provider:   Cloudflare (from prefix rule)
  CDN Provider:   Cloudflare (from prefix rule)
  nginx template: ws.conf.tmpl (from prefix rule)
  HTML template:  none (from prefix rule)
```

Prefix rules have two levels: system-wide defaults + project-level overrides.
Project-level ALWAYS wins.

---

## 2. Subsystem Responsibilities

### 2.1 Domain Registry & Configuration (internal/domain, internal/project)

- CRUD for projects, main domains, subdomains, prefix rules
- Automatic subdomain generation from prefix rules when a domain is registered
- Domain state machine enforcement (see CLAUDE.md)
- Audit trail for all state transitions

### 2.2 DNS/CDN Automation (pkg/provider)

- Unified interface abstracting 5 CDN + 4 DNS vendors
- Provider registry with runtime lookup by name
- All provider operations are async (dispatched as asynq tasks)
- Retry with exponential backoff per provider
- Provider-specific quirks handled INSIDE the implementation, NEVER leaked to business logic

**Provider implementation priority:**
- P0: Cloudflare (DNS+CDN), Alibaba Cloud (DNS+CDN)
- P1: Tencent Cloud (DNS+CDN), GoDaddy (DNS)
- P2: Huawei Cloud (DNS+CDN), Self-hosted CDN

### 2.3 Release Subsystem (internal/release)

Hierarchy:
```
Release
  └── Shard (200-500 domains per shard)
       └── DomainTask (one per domain: render → deploy → verify)
            └── ServerTask (one per target machine: svn up → nginx reload)
```

**Canary strategy:**
- Deploy Shard 1 → wait for probe verification → success rate ≥ 95% → continue
- Success rate < 95% → auto-pause Release, alert operators
- Any shard can be paused/resumed/rolled back independently

**nginx reload aggregation:**
- Same server, multiple conf changes → buffer 30 seconds OR 50 domains, then single reload
- Emergency (P1 alert failover) → skip buffer, immediate reload
- ALWAYS run `nginx -t` before reload. Fail → rollback ALL conf changes for this batch.

### 2.4 Probe Monitoring (internal/probe, cmd/scanner)

**Three-tier probing:**

| Tier | Target | Checks | Frequency | Concurrency |
|------|--------|--------|-----------|-------------|
| L1 | All 12K main domains | DNS + TCP :443 | Every 90s | 500 goroutines |
| L2 | L1-passed domains, sample 1-2 subdomains | HTTP 200 + latency | Every 5min | 200 goroutines |
| L3 | Manually tagged core domains | HTTP + keyword + TLS expiry | Every 30s | 50 goroutines |

**Block detection logic:**
```
DNS poisoning:   Resolved IPs match known GFW poison IPs (127.0.0.1, 243.185.187.39, etc.)
TCP block:       connect() to :443 times out after 3s
SNI block:       TCP connects but TLS handshake fails
HTTP hijack:     Response contains block keywords OR unexpected redirect
Content tamper:  Response body checksum mismatch
```

**Data flow:**
```
CN Probe Nodes (Go Scanner)
    │
    │ HTTPS POST /api/v1/probe/push (mTLS authenticated)
    ▼
Probe Receiver (境外)
    │
    ├──→ TimescaleDB (raw results, 90-day retention)
    ├──→ Redis (current state per domain, dedup)
    └──→ Alert Engine (on state change only)
```

### 2.5 Alert & Auto-Disposition (internal/alert, internal/switcher)

**Alert severity:**

| Level | Trigger | Auto-action |
|-------|---------|-------------|
| P0 | Standby pool exhausted / entire project unreachable | Pause all releases |
| P1 | Main domain blocked (DNS poison / TCP block / HTTP hijack) | Trigger auto-switch |
| P2 | Partial subdomain anomaly / pool < 5 remaining | Alert for manual intervention |
| P3 | High latency / non-critical anomaly | Log only |
| INFO | Domain recovered / switch completed | Notification |

**Auto-switch flow (P1 trigger):**
```
1. Send alert (Telegram + Webhook)
2. Pause all pending releases for this domain's project
3. Select highest-priority standby domain from pool
4. DNS migration: delete old CNAMEs, create new CNAMEs
5. CDN migration: clone config from old domain to new domain
6. Re-render nginx conf with new main domain (prefixes unchanged)
7. SVN commit + Agent deploy
8. Wait 30-60s for DNS TTL + CDN propagation
9. Probe verification from all CN nodes
10. Update DB: old domain → blocked, new domain → active
```

Each step has rollback logic. If step N fails, steps 1..N-1 are reversed.

### 2.6 Standby Pool (internal/pool)

**Lifecycle:** `pending → standby → active → blocked → retired`

**Pre-warming (required before standby):**
1. Create DNS CNAME records for all prefixes
2. Create CDN configurations for all prefixes
3. Wait for DNS + CDN propagation
4. Verify reachability from ALL 6 CN probe nodes
5. ALL pass → status = standby. ANY fail → status = pending, alert.

**Pool thresholds:**
- Normal domains: alert at < 2 remaining
- Core domains: alert at < 5 remaining
- Any domain pool = 0: P0 alert (critical)

---

## 3. Data Architecture

### PostgreSQL Tables

All tables follow these conventions:
- `id BIGSERIAL PRIMARY KEY` — internal use only, never exposed in API
- `uuid UUID NOT NULL DEFAULT gen_random_uuid()` — external identifier
- `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `deleted_at TIMESTAMPTZ` — soft delete

Key relationships:
```
projects 1──N main_domains
main_domains 1──N subdomains
main_domains 1──N domain_state_history
projects 1──N main_domain_pool
projects 1──N releases
releases 1──N release_shards
release_shards 1──N domain_tasks
main_domains 1──N switch_history
```

### TimescaleDB (probe_results hypertable)

```sql
CREATE TABLE probe_results (
    probe_node  VARCHAR(32) NOT NULL,
    isp         VARCHAR(16) NOT NULL,
    domain      VARCHAR(253) NOT NULL,
    status      VARCHAR(16) NOT NULL,
    dns_ok      BOOLEAN NOT NULL,
    dns_ips     TEXT[],
    tcp_latency_ms FLOAT,
    http_code   SMALLINT,
    http_hijacked BOOLEAN,
    checked_at  TIMESTAMPTZ NOT NULL
);

SELECT create_hypertable('probe_results', 'checked_at');

-- Retention policy: 90 days raw, aggregated summaries permanent
SELECT add_retention_policy('probe_results', INTERVAL '90 days');

-- Continuous aggregate for dashboard
CREATE MATERIALIZED VIEW probe_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', checked_at) AS bucket,
    domain,
    probe_node,
    COUNT(*) AS total_checks,
    COUNT(*) FILTER (WHERE status = 'ok') AS ok_count,
    AVG(tcp_latency_ms) AS avg_latency
FROM probe_results
GROUP BY bucket, domain, probe_node;
```

### Redis Key Design

```
# Domain current status (for alert dedup)
domain:status:{probe_node}:{domain} → "ok" | "dns_poison" | "tcp_block" | ...
TTL: 3600s

# Alert dedup
alert:dedup:{probe_node}:{domain}:{status} → "1"
TTL: 3600s (same status won't re-alert within 1 hour)

# nginx reload buffer
reload:buffer:{server_id} → SET of domain_task_ids
TTL: 60s (auto-flush if not manually triggered)
```

---

## 4. Communication Security

### Probe ↔ Controller: mTLS

```
CN Probe Node                    Probe Controller
┌──────────┐    TLS 1.3 mTLS    ┌──────────────┐
│ Client   │ ──────────────────→ │ Server       │
│ Cert     │                     │ Cert         │
│ (unique) │                     │ (controller) │
└──────────┘                     └──────────────┘
     │                                  │
     └──── Both signed by ─────────────┘
           Internal CA
```

- Each probe gets a unique client certificate signed by internal CA
- Controller validates client cert against CA root — rejects unknown probes
- Certificate rotation: every 90 days, 7-day overlap grace period
- Failed auth: reject immediately, do NOT return error details

### Management Console: JWT + HTTPS

- Caddy handles HTTPS (auto Let's Encrypt)
- JWT tokens: 24h expiry, HS256 signed
- Password storage: bcrypt, cost factor 12
- Rate limiting: login endpoint 10 req/min

---

## 5. Deployment Topology

### Phase 1 (5 machines)

```
┌─── Taiwan ─────────────────────────────────────┐
│                                                 │
│  Main Node (8C/32G SSD)                        │
│  ┌─────────────────────────────────────┐       │
│  │ Caddy (reverse proxy + static)      │       │
│  │ domain-platform (Gin API :8080)     │       │
│  │ domain-worker (asynq worker)        │       │
│  │ PostgreSQL 16 + TimescaleDB (:5432) │       │
│  │ Redis 7 (:6379)                     │       │
│  └─────────────────────────────────────┘       │
│                                                 │
│  Probe Controller (2C/4G)                      │
│  ┌─────────────────────────────────────┐       │
│  │ domain-platform probe-receiver      │       │
│  │ Alert Engine + Telegram Bot         │       │
│  │ Auto-Switch Engine                  │       │
│  └─────────────────────────────────────┘       │
│                                                 │
└─────────────────────────────────────────────────┘

┌─── Mainland China ─────────────────────────────┐
│  cn-probe-ct (Telecom, 1C/1G)  → Go Scanner   │
│  cn-probe-cu (Unicom,  1C/1G)  → Go Scanner   │
│  cn-probe-cm (Mobile,  1C/1G)  → Go Scanner   │
└─────────────────────────────────────────────────┘
```

### Phase 2 Expansion

- Separate Deploy Worker (8C/16G) for batch release CPU-intensive work
- CN probes: 3 → 6 (2 per ISP, north + south coverage)
- Consider ClickHouse migration if TimescaleDB queries degrade

---

## 6. Build & Deploy

### Build Artifacts

```bash
# API + Web server
GOOS=linux GOARCH=amd64 go build -o bin/domain-platform ./cmd/server

# Task worker
GOOS=linux GOARCH=amd64 go build -o bin/domain-worker ./cmd/worker

# Probe scanner (for CN nodes)
GOOS=linux GOARCH=amd64 go build -o bin/domain-scanner ./cmd/scanner

# DB migration tool
GOOS=linux GOARCH=amd64 go build -o bin/domain-migrate ./cmd/migrate

# Vue frontend
cd web && npm run build  # outputs to web/dist/
```

### Deployment Process

```bash
# 1. Build
make build && make web

# 2. Upload
scp bin/domain-platform bin/domain-worker user@main-node:/opt/domain-platform/
scp -r web/dist/ user@main-node:/opt/domain-platform/web/dist/
scp bin/domain-scanner user@cn-probe-ct:/opt/domain-scanner/

# 3. Migrate
ssh main-node "/opt/domain-platform/domain-migrate up"

# 4. Restart
ssh main-node "sudo systemctl restart domain-platform domain-worker"
ssh cn-probe-ct "sudo systemctl restart domain-scanner"

# 5. Verify
curl https://platform.example.com/health
```
