# PHASE_B_TASKLIST.md — DNS Operations Work Order

> **Created 2026-04-21.** This document is the authoritative work order for
> Phase B (DNS Operations) of the platform restructuring.
>
> **Pre-requisite**: Phase A complete (PA.1–PA.3 minimum: schema, registrars,
> dns_providers, domain asset extension). Read CLAUDE.md, ARCHITECTURE.md,
> ARCHITECTURE_ROADMAP.md §5, and the relevant analysis docs before starting.
>
> **Audience**: Claude Code sessions (Opus for sync engine + safety logic,
> Sonnet for CRUD/UI tasks).

---

## Phase B — Definition of Scope

Phase B transforms the platform from "knows where DNS is hosted" (Phase A)
into "manages what DNS records SHOULD exist and syncs them safely to
providers" — a declarative DNS management layer inspired by DNSControl and
OctoDNS.

### What "Phase B done" looks like (acceptance demo)

```
1. Admin navigates to domain "example.com" → DNS tab
2. Adds records: A @ → 1.2.3.4 TTL 300, CNAME www → example.com, MX @ → mail.example.com
3. Records are saved as DESIRED STATE in the platform DB
4. Admin clicks "Plan" → system fetches actual records from Cloudflare,
   computes diff: 2 creates, 0 updates, 0 deletes
5. Plan shows safety check: ✅ (only 2 changes on zone with 5 existing records)
6. Admin clicks "Apply" → system executes corrections via Cloudflare API
7. dns_sync_history records: who, when, what changed (before/after)
8. Next day: scheduled drift detection runs → detects someone added a TXT
   record directly in Cloudflare → shows as "unmanaged record" in UI
9. Admin creates a new domain → applies DNS template "Standard Web" →
   pre-populates A + CNAME + MX + SPF + DMARC records
10. Admin with "viewer" role can see records but cannot edit or apply
```

### What is OUT of Phase B (do not implement)

| Feature | Phase | Reason |
|---|---|---|
| Multi-provider sync (same zone → 2+ providers) | Future | Start with 1 provider per domain |
| DNSSEC key management | Future | Complex; defer to provider native tools |
| GeoDNS / weighted routing | Future | Provider-specific; no generic abstraction yet |
| Auto-remediation (drift → auto-fix) | Phase C | Requires alert engine for safe automation |
| DNS-based failover (switch on blocking) | Phase D | Requires GFW detection signal |
| Terraform/IaC export | Future | Nice-to-have, not core |
| Reverse DNS (PTR) management | Future | Different workflow |
| DNS propagation checking | Phase C | Part of probe/monitoring |

---

## Dependency Graph

```
    PB.1 (DNS Record Data Model — tables + models + store)
       │
       ├──────────────────┐
       ▼                  ▼
    PB.2               PB.6
  Provider Sync      Zone RBAC
  Engine (Opus)      (independent)
       │
       ├──────────────────┐
       ▼                  ▼
    PB.3               PB.4
  Plan/Apply         Safety
  Workflow           Thresholds
       │                  │
       └────────┬─────────┘
                ▼
             PB.5
          DNS Mgmt UI
                │
                ▼
             PB.7
        DNS Templates +
        Drift Detection
```

### Critical path

`PB.1 → PB.2 → PB.3 → PB.5`

### Parallelization rules

- PB.2 and PB.6 can run in parallel after PB.1
- PB.3 and PB.4 can start after PB.2 (PB.4 is safety logic embedded in PB.3's workflow)
- PB.5 depends on PB.3 (needs plan/apply API) and PB.4 (safety display)
- PB.7 depends on PB.5 (needs the DNS UI base to add templates + drift view)

---

## Task Cards

---

### PB.1 — DNS Record Data Model **(Opus)**

**Owner**: **Opus** — schema design sets the foundation for sync correctness
**Depends on**: Phase A complete (PA.1 schema, PA.2 dns_providers table)
**Reads first**: `docs/analysis/DNSCONTROL_ANALYSIS.md` §5 "Proposed Interface",
`docs/analysis/OCTODNS_ANALYSIS.md` §4 "Zone Model" + §5 "Record Model",
`docs/analysis/POWERDNS_ADMIN_ANALYSIS.md` §2 "Domain Template"

**Context**: The platform needs to store DNS records as "desired state" (what
SHOULD exist at the provider). This is separate from the provider's actual
state — the diff between desired and actual is what drives plan/apply.

**Scope (in)**:

- New table `dns_records` (desired state — source of truth in our DB):
  ```sql
  CREATE TABLE dns_records (
      id              BIGSERIAL PRIMARY KEY,
      uuid            UUID NOT NULL DEFAULT gen_random_uuid(),
      domain_id       BIGINT NOT NULL REFERENCES domains(id),
      name            VARCHAR(255) NOT NULL,        -- "@", "www", "mail", "_dmarc"
      type            VARCHAR(16) NOT NULL,         -- A, AAAA, CNAME, MX, TXT, SRV, CAA, NS
      content         TEXT NOT NULL,                -- target value
      ttl             INT NOT NULL DEFAULT 300,
      priority        INT,                          -- MX/SRV priority
      extra           JSONB DEFAULT '{}',           -- type-specific: SRV weight/port, CAA flags, etc.
      managed         BOOLEAN NOT NULL DEFAULT true, -- false = record exists but platform doesn't manage it
      provider_record_id VARCHAR(128),              -- provider's ID for update/delete operations
      comment         TEXT,
      created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      deleted_at      TIMESTAMPTZ,
      UNIQUE(domain_id, name, type, content) WHERE deleted_at IS NULL
  );

  CREATE INDEX idx_dns_records_domain ON dns_records(domain_id);
  CREATE INDEX idx_dns_records_type ON dns_records(type);
  ```

- New table `dns_sync_history` (audit trail for plan/apply):
  ```sql
  CREATE TABLE dns_sync_history (
      id              BIGSERIAL PRIMARY KEY,
      domain_id       BIGINT NOT NULL REFERENCES domains(id),
      action          VARCHAR(32) NOT NULL,          -- "plan", "apply", "drift_detected"
      plan_hash       VARCHAR(64),                   -- SHA-256 of plan for verification
      changes         JSONB NOT NULL,                -- [{type:"create",record:{...}}, ...]
      change_count    INT NOT NULL DEFAULT 0,
      safety_result   VARCHAR(32),                   -- "passed", "threshold_exceeded", "force_applied"
      applied_by      BIGINT REFERENCES users(id),
      applied_at      TIMESTAMPTZ,
      error           TEXT,
      created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );

  CREATE INDEX idx_dns_sync_domain ON dns_sync_history(domain_id);
  ```

- New table `dns_record_templates`:
  ```sql
  CREATE TABLE dns_record_templates (
      id              BIGSERIAL PRIMARY KEY,
      name            VARCHAR(128) NOT NULL UNIQUE,  -- "Standard Web", "Email Only", "Bare Minimum"
      description     TEXT,
      records         JSONB NOT NULL,                -- [{name:"@",type:"A",content:"{{ip}}",ttl:300}, ...]
      variables       JSONB DEFAULT '{}',            -- template variables: {"ip": "", "mx_host": ""}
      created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  ```

- New table `domain_permissions` (zone-level RBAC, used by PB.6):
  ```sql
  CREATE TABLE domain_permissions (
      id              BIGSERIAL PRIMARY KEY,
      domain_id       BIGINT NOT NULL REFERENCES domains(id),
      user_id         BIGINT NOT NULL REFERENCES users(id),
      permission      VARCHAR(32) NOT NULL DEFAULT 'viewer', -- viewer, editor, admin
      granted_by      BIGINT REFERENCES users(id),
      granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      UNIQUE(domain_id, user_id)
  );
  ```

- Extend `domains` table (in-place edit, still pre-launch):
  ```sql
  ALTER TABLE domains ADD COLUMN purge_unmanaged BOOLEAN NOT NULL DEFAULT false;
  ALTER TABLE domains ADD COLUMN dns_sync_enabled BOOLEAN NOT NULL DEFAULT false;
  ALTER TABLE domains ADD COLUMN last_sync_at TIMESTAMPTZ;
  ALTER TABLE domains ADD COLUMN last_drift_at TIMESTAMPTZ;
  ```

- Extend `dns_providers` table:
  ```sql
  ALTER TABLE dns_providers ADD COLUMN sync_config JSONB DEFAULT '{
      "update_threshold_pct": 30,
      "delete_threshold_pct": 30,
      "min_existing_records": 10
  }';
  ```

- Go models:
  - `internal/dnsrecord/model.go`: `DNSRecord`, `DNSSyncHistory`, `DNSRecordTemplate`
  - `pkg/provider/dns/types.go`: `Record`, `Correction`, `CorrectionType`, `Plan`
    (from DNSCONTROL_ANALYSIS.md §5 "Proposed Interface")

- Store implementations:
  - `store/postgres/dnsrecord.go`: CRUD for dns_records, dns_sync_history, dns_record_templates
  - Methods: `ListByDomain`, `Upsert`, `BulkUpsert`, `SoftDelete`, `GetUnmanaged`,
    `CreateSyncHistory`, `GetLatestSync`

**Scope (out)**:

- Provider sync logic (PB.2)
- Plan/apply API (PB.3)
- Frontend (PB.5)
- Drift detection worker (PB.7)

**Deliverables**:

- Migration SQL (4 new tables + 2 table extensions)
- Go model structs with proper tags
- `pkg/provider/dns/types.go` — Record, Correction, Plan types
- Store implementations (CRUD)
- `make migrate-up` succeeds
- `go build ./...` + `go test ./store/postgres/...` passes

**Acceptance**:

- All tables created with correct constraints + indexes
- `dns_records` UNIQUE partial index works (allows soft-deleted duplicates)
- Can insert A, AAAA, CNAME, MX, TXT, SRV, CAA records with correct `extra` JSONB
- `dns_sync_history` stores plan/apply audit correctly
- `dns_record_templates` JSONB stores template records + variables
- `domain_permissions` enforces UNIQUE(domain_id, user_id)
- Foreign keys work (can't insert record for non-existent domain)
- `go test ./store/postgres/...` passes

---

### PB.2 — Provider Sync Engine **(Opus)**

**Owner**: **Opus** — most critical correctness path in Phase B; bugs here
corrupt production DNS
**Depends on**: PB.1 (data model + `pkg/provider/dns/types.go`)
**Reads first**: `docs/analysis/DNSCONTROL_ANALYSIS.md` §2 "Provider Interfaces"
+ §5 "Proposed Interface", `docs/analysis/OCTODNS_ANALYSIS.md` §2 "BaseProvider"
+ §3 "Plan & Safety"

**Context**: The sync engine is the core of Phase B. It fetches actual state
from a DNS provider, compares with desired state in our DB, computes a diff
(corrections), and can execute those corrections. This is the "brain" that
DNSControl and OctoDNS both implement.

**Scope (in)**:

- `pkg/provider/dns/provider.go` — finalize the interface:
  ```go
  type Provider interface {
      Name() string
      GetZoneRecords(ctx context.Context, zone string) ([]Record, error)
      PlanChanges(ctx context.Context, zone string, desired, existing []Record) (*Plan, error)
      ApplyCorrections(ctx context.Context, zone string, corrections []Correction) error
  }
  ```

- `pkg/provider/dns/plan.go` — Plan type with diff computation:
  ```go
  type Plan struct {
      Zone        string
      Creates     []Correction
      Updates     []Correction
      Deletes     []Correction
      Unchanged   int
      Hash        string  // SHA-256 for checksum verification
  }

  func ComputePlan(desired, existing []Record, purgeUnmanaged bool) *Plan
  ```
  - Match records by (name, type, content) tuple
  - Detect creates (in desired, not in existing)
  - Detect updates (in both, but TTL or extra differ)
  - Detect deletes (in existing, not in desired, AND purgeUnmanaged=true)
  - Mark unmanaged records (in existing, not in desired, purgeUnmanaged=false)
  - Compute plan hash: `sha256(sorted JSON of all corrections)`

- `pkg/provider/dns/correction.go` — Correction type:
  ```go
  type Correction struct {
      Type        CorrectionType  // create, update, delete
      Description string          // human-readable
      Record      Record          // the target record
      OldRecord   *Record         // previous state (for updates)
  }
  ```

- `pkg/provider/dns/cloudflare/` — Cloudflare implementation:
  - Extend existing `pkg/provider/dns/cloudflare.go` (from P2.7 stub)
  - `GetZoneRecords()`: GET /zones/{zone_id}/dns_records → map to []Record
  - `PlanChanges()`: call `ComputePlan()` with provider-specific filtering
    (skip provider-managed records like auto-generated CF proxy entries)
  - `ApplyCorrections()`:
    - Create: POST /zones/{zone_id}/dns_records
    - Update: PUT /zones/{zone_id}/dns_records/{id}
    - Delete: DELETE /zones/{zone_id}/dns_records/{id}
  - Zone ID resolution: lookup from `dns_providers.config` JSONB
    `{"zones": {"example.com": "zone_id_here"}}`
  - Use `github.com/cloudflare/cloudflare-go/v2` SDK

- `pkg/provider/dns/registry.go` — provider registry:
  ```go
  func Register(name string, factory ProviderFactory)
  func GetProvider(name string, config, credentials json.RawMessage) (Provider, error)
  ```
  - Register "cloudflare" at init
  - Factory creates provider instance from config + credentials

- `internal/dnssync/service.go` — orchestration service:
  ```go
  type Service struct {
      records   DNSRecordStore
      providers DNSProviderStore
      registry  *dns.Registry
      redis     *redis.Client
      logger    *zap.Logger
  }

  func (s *Service) FetchExisting(ctx, domainID) ([]dns.Record, error)
  func (s *Service) ComputePlan(ctx, domainID) (*dns.Plan, error)
  func (s *Service) ApplyPlan(ctx, domainID, planHash, userID) error
  func (s *Service) RecordDrift(ctx, domainID) (*DriftReport, error)
  ```

**Scope (out)**:

- Safety thresholds (PB.4 — but `ComputePlan` returns the data needed)
- API handlers (PB.3)
- Scheduled drift detection (PB.7)
- Multiple providers per domain (future)
- Route53, PowerDNS implementations (future — add as needed)

**Deliverables**:

- Finalized `pkg/provider/dns/` interface + types
- `ComputePlan()` diff engine with unit tests
- Cloudflare provider implementation (all 3 interface methods)
- Provider registry
- `internal/dnssync/service.go` orchestration
- Integration test: seed desired records → plan → apply → verify at provider

**Acceptance**:

- `ComputePlan()` with 5 desired + 5 existing (2 same, 1 updated TTL, 2 new desired, 2 existing-only):
  → creates=2, updates=1, deletes=0 (purge=false), unchanged=2
- Same with `purgeUnmanaged=true` → deletes=2
- Cloudflare `GetZoneRecords()` returns correct records from API
- Cloudflare `ApplyCorrections()` creates/updates/deletes records at Cloudflare
- Plan hash is deterministic (same input → same hash)
- Plan hash changes when corrections change
- Provider registry: `GetProvider("cloudflare", config, creds)` returns working instance
- `go test -race ./pkg/provider/dns/... ./internal/dnssync/...` passes
- Error handling: Cloudflare API 429 (rate limit) → retry with backoff
- Error handling: invalid record type → clear error message, no partial apply

---

### PB.3 — Plan/Apply Workflow API

**Owner**: Sonnet
**Depends on**: PB.2 (sync engine must work end-to-end)
**Reads first**: `docs/analysis/OCTODNS_ANALYSIS.md` §6 "Manager — Sync
Orchestration", §7 "Proposed Plan/Apply API Flow"

**Context**: The plan/apply pattern ensures no DNS changes happen without
explicit review. Plan computes what WOULD change; apply executes with a
checksum guard to prevent stale plans from being applied.

**Scope (in)**:

- `api/handler/dnssync.go`:
  - `POST /api/v1/domains/:id/dns/plan`:
    - Call `dnssync.Service.ComputePlan(ctx, domainID)`
    - Store plan hash in Redis: `dns:plan:{domain_id}` → hash, TTL 1 hour
    - Return: plan (creates/updates/deletes with human-readable descriptions),
      safety result, plan_hash
    - Record in `dns_sync_history` (action="plan")
  - `POST /api/v1/domains/:id/dns/apply`:
    - Body: `{ "plan_hash": "sha256:...", "force": false }`
    - Verify hash matches Redis-stored plan (reject if stale/mismatched)
    - If safety thresholds exceeded AND `force=false` → return 409 with explanation
    - If safety passes OR `force=true` → call `dnssync.Service.ApplyPlan()`
    - Record in `dns_sync_history` (action="apply", changes, applied_by)
    - Return: applied corrections + any errors
  - `GET /api/v1/domains/:id/dns/sync-status`:
    - Return: last_sync_at, last_drift_at, unmanaged record count,
      current plan (if any in Redis)

- `api/handler/dnsrecord.go` — desired state CRUD:
  - `GET /api/v1/domains/:id/dns/records` — list desired records
  - `POST /api/v1/domains/:id/dns/records` — create desired record
  - `PUT /api/v1/domains/:id/dns/records/:id` — update desired record
  - `DELETE /api/v1/domains/:id/dns/records/:id` — soft delete desired record
  - `POST /api/v1/domains/:id/dns/records/bulk` — bulk create/update
    (for template application + CSV import)

- Redis plan storage:
  - Key: `dns:plan:{domain_id}`
  - Value: JSON `{ "hash": "sha256:...", "plan": {...}, "computed_at": "...", "user_id": 123 }`
  - TTL: 3600 seconds (1 hour)
  - On apply: delete key after successful execution

- asynq task `dns:sync`:
  - For each domain where `dns_sync_enabled = true`:
    - Compute plan → if changes detected → record drift
    - Update `domains.last_sync_at`
    - If drift found → update `domains.last_drift_at`
  - Runs every 6 hours (configurable)
  - Does NOT auto-apply — only detects and records

- Route registration + middleware:
  - DNS record write operations require `editor` permission or above
  - DNS apply requires `admin` or `release_manager` role
  - Plan (read-only) requires `viewer` or above

**Scope (out)**:

- Safety threshold logic (PB.4 — called from within apply handler)
- Frontend (PB.5)
- Drift alerting (PB.7 — this task only records drift)

**Deliverables**:

- Plan/apply API endpoints
- DNS record CRUD endpoints
- Redis plan caching with checksum verification
- dns:sync asynq worker (drift detection)
- Route registration with permission checks
- `dns_sync_history` audit trail for all plan/apply actions

**Acceptance**:

- `POST /api/v1/domains/:id/dns/plan` returns plan with hash
- Same domain, same desired state → same hash (deterministic)
- `POST /api/v1/domains/:id/dns/apply` with correct hash → executes
- Apply with wrong hash → 409 "plan has changed or expired"
- Apply after 1 hour (TTL expired) → 409 "plan expired, re-plan required"
- Apply with `force=false` + safety exceeded → 409 with threshold details
- Apply with `force=true` + safety exceeded → executes (with audit note)
- DNS record CRUD: create A record → visible in list → included in next plan
- `dns:sync` worker runs → detects externally added record → `last_drift_at` updated
- `dns_sync_history` contains complete audit trail
- Non-editor user tries to create record → 403
- Non-admin user tries to apply → 403
- `go test ./api/handler/... ./internal/dnssync/...` passes

---

### PB.4 — Safety Thresholds **(Opus)**

**Owner**: **Opus** — safety logic must be bulletproof; bugs here = DNS outage
**Depends on**: PB.2 (plan computation provides the data), PB.3 (apply handler calls safety check)
**Reads first**: `docs/analysis/OCTODNS_ANALYSIS.md` §3 "Plan & Safety Thresholds"

**Context**: OctoDNS refuses to execute plans that would delete/update more
than 30% of existing records (unless forced). This prevents accidental zone
wipes. We adopt this pattern with configurable thresholds per provider.

**Scope (in)**:

- `pkg/provider/dns/safety.go`:
  ```go
  type SafetyConfig struct {
      UpdateThresholdPct  float64  // default 0.3 (30%)
      DeleteThresholdPct  float64  // default 0.3 (30%)
      MinExistingRecords  int      // default 10 (skip safety on small zones)
      ProtectRootNS       bool     // default true (NS @ changes always require force)
  }

  type SafetyResult struct {
      Passed           bool
      Reason           string   // "" if passed
      UpdatePct        float64  // actual percentage
      DeletePct        float64
      UpdateThreshold  float64  // configured threshold
      DeleteThreshold  float64
      ExistingCount    int
      RootNSChanged    bool
      RequiresForce    bool
  }

  func CheckSafety(plan *Plan, existing []Record, config SafetyConfig) *SafetyResult
  ```

- Safety check logic (from OctoDNS, exact):
  1. If `len(existing) < MinExistingRecords` → PASS (small zone, no safety needed)
  2. `updatePct = len(plan.Updates) / len(existing)`
     If `updatePct > UpdateThresholdPct` → FAIL "would update {n}% of records (threshold: {t}%)"
  3. `deletePct = len(plan.Deletes) / len(existing)`
     If `deletePct > DeleteThresholdPct` → FAIL "would delete {n}% of records (threshold: {t}%)"
  4. If `ProtectRootNS && plan contains NS record at "@"` → FAIL "root NS change requires force"
  5. Otherwise → PASS

- Integration into apply workflow:
  - `api/handler/dnssync.go` apply handler calls `CheckSafety()` before executing
  - If safety fails AND `force=false` → return 409 with `SafetyResult` JSON
  - If safety fails AND `force=true` → proceed but record `safety_result="force_applied"` in audit
  - If safety passes → proceed normally, record `safety_result="passed"`

- Safety config loading:
  - Load from `dns_providers.sync_config` JSONB for the domain's provider
  - Fallback to defaults if not configured
  - API to update: `PUT /api/v1/dns-providers/:id` (sync_config field)

- Additional guardrails:
  - Cannot apply empty plan (0 changes) → return 400 "nothing to apply"
  - Cannot apply to domain where `dns_sync_enabled = false` → return 409
  - Cannot apply if domain has no `dns_provider_id` → return 409
  - Rate limit: max 1 apply per domain per 5 minutes (Redis key)

**Scope (out)**:

- Dynamic threshold adjustment based on zone size (future)
- Machine learning anomaly detection (way future)
- Provider-specific safety rules (e.g., Cloudflare proxy changes)

**Deliverables**:

- `pkg/provider/dns/safety.go` with `CheckSafety()`
- Integration into apply handler
- Per-provider configurable thresholds via `sync_config` JSONB
- Additional guardrails (rate limit, empty plan rejection)
- Comprehensive unit tests for all safety scenarios

**Acceptance**:

- Zone with 20 records, plan deletes 8 → `SafetyResult.Passed=false`, reason contains "40%"
- Zone with 20 records, plan deletes 5 → `SafetyResult.Passed=true`
- Zone with 5 records, plan deletes 4 → `SafetyResult.Passed=true` (below MIN_EXISTING)
- Plan changes NS @ record → `SafetyResult.RootNSChanged=true`, requires force
- Apply with force=true + safety failed → executes, audit says "force_applied"
- Custom threshold: provider sync_config has `delete_threshold_pct: 0.5` → uses 50%
- Rate limit: two applies within 5 minutes → second returns 429
- `go test -race ./pkg/provider/dns/...` with safety test cases passes

---

### PB.5 — DNS Management UI

**Owner**: Sonnet
**Depends on**: PB.3 (plan/apply API), PB.4 (safety result display)
**Reads first**: `docs/analysis/POWERDNS_ADMIN_ANALYSIS.md` §4 "Key Design
Patterns" (staged-edit + batch-apply), `docs/FRONTEND_GUIDE.md`

**Context**: The DNS UI follows PowerDNS-Admin's "staged-edit + batch-apply"
pattern: users edit records in-memory, preview changes, then submit as one
batch. The plan/apply workflow gives them a clear diff before execution.

**Scope (in)**:

- `web/src/views/domains/DomainDNS.vue` — DNS tab in domain detail:
  - Record table: columns = name, type, content, TTL, priority, managed, actions
  - Sortable by name, type
  - Filter by type (dropdown: All, A, AAAA, CNAME, MX, TXT, etc.)
  - Status indicator: "In sync" | "Pending changes" | "Drift detected"

- Inline record editing (staged, not immediate):
  - Click row → inline edit mode (name, type, content, TTL, priority)
  - Type-specific validation:
    - A: IPv4 format
    - AAAA: IPv6 format
    - CNAME: hostname format, cannot coexist with other types at same name
    - MX: requires priority + hostname
    - TXT: any string (max 255 per segment)
    - SRV: priority + weight + port + target
    - CAA: flags + tag + value
  - Add new record row (inline)
  - Delete record (mark for deletion, show strikethrough)
  - All edits are LOCAL (in Pinia store) until submitted

- Staged changes panel (appears when edits exist):
  - Shows: N added, N modified, N deleted
  - "Discard All" button → revert to server state
  - "Save to Platform" button → bulk submit to desired state API
    (`POST /api/v1/domains/:id/dns/records/bulk`)

- Plan/Apply workflow UI:
  - "Plan" button → calls plan API → shows plan modal:
    - Creates (green): record details
    - Updates (yellow): old → new diff
    - Deletes (red): record being removed
    - Safety status: ✅ passed or ⚠️ threshold exceeded (with details)
    - Plan hash displayed (for reference)
  - "Apply" button in modal → calls apply API
    - Shows spinner during execution
    - On success: "Applied N changes" notification + refresh record list
    - On failure: show error details
  - "Force Apply" button (only shown if safety failed + user is admin)

- Unmanaged records display:
  - Records from provider not in desired state shown in gray/italic
  - Tooltip: "This record exists at provider but is not managed by platform"
  - Action: "Import" (adds to desired state) or "Mark for deletion"

- `web/src/api/dnsrecord.ts`:
  - `list(domainId)`, `create(domainId, record)`, `update(id, record)`,
    `delete(id)`, `bulkUpsert(domainId, records)`
  - `plan(domainId)`, `apply(domainId, planHash, force)`
  - `syncStatus(domainId)`

- `web/src/stores/dnsrecord.ts` — Pinia store:
  - `records: DNSRecord[]` (server state)
  - `pendingChanges: PendingChange[]` (local edits)
  - `hasPendingChanges: boolean` (computed)
  - `plan: Plan | null`
  - Actions: `fetchRecords`, `addRecord`, `editRecord`, `deleteRecord`,
    `discardChanges`, `saveToPlatform`, `computePlan`, `applyPlan`

- `web/src/types/dnsrecord.ts`:
  - `DNSRecord`, `Plan`, `Correction`, `SafetyResult`, `SyncStatus`

**Scope (out)**:

- Rich diff viewer (side-by-side) — monospace text diff is sufficient
- Drag-and-drop record reordering
- Record import from zone file
- Batch operations across multiple domains
- Real-time collaboration (multiple editors)

**Deliverables**:

- DNS tab component with record table + inline editing
- Staged-edit pattern with Pinia store
- Plan modal with create/update/delete display + safety status
- Apply execution with success/error handling
- Unmanaged record display
- Type-specific validation
- `npm run build` clean

**Acceptance**:

- Add A record → shows in "pending changes" → save → record persisted
- Edit TTL → shows as modified → plan → update shown in yellow
- Delete record → strikethrough → plan → delete shown in red
- Plan shows safety check ✅ → apply → changes executed at Cloudflare
- Safety threshold exceeded → warning shown → "Force Apply" only for admin
- Unmanaged record appears in gray → click "Import" → becomes managed
- CNAME validation: reject CNAME at same name as existing A record
- MX without priority → validation error
- Discard all → reverts to server state
- `npm run build` succeeds with zero TypeScript errors

---

### PB.6 — Zone-Level RBAC

**Owner**: Sonnet
**Depends on**: PB.1 (domain_permissions table)
**Reads first**: `docs/analysis/POWERDNS_ADMIN_ANALYSIS.md` §3 "RBAC Pattern"

**Context**: Phase 1 RBAC is role-based (admin/release_manager/operator/viewer)
and project-scoped. Phase B adds domain-level permissions so specific users can
manage DNS for specific domains regardless of their global role.

**Scope (in)**:

- `internal/domain/permission.go`:
  ```go
  func (s *Service) GrantPermission(ctx, domainID, userID int64, permission string, grantedBy int64) error
  func (s *Service) RevokePermission(ctx, domainID, userID int64) error
  func (s *Service) GetPermission(ctx, domainID, userID int64) (string, error)
  func (s *Service) ListPermissions(ctx, domainID int64) ([]DomainPermission, error)
  func (s *Service) HasPermission(ctx, domainID, userID int64, minLevel string) (bool, error)
  ```

- Permission levels (ordered):
  - `viewer` — can see records, can view plan results, cannot edit
  - `editor` — can edit desired records, can trigger plan, cannot apply
  - `admin` — can edit, plan, apply, grant/revoke permissions

- Access resolution (two-path, from PowerDNS-Admin):
  ```go
  func (s *Service) CanAccess(ctx, domainID, userID int64, requiredLevel string) (bool, error) {
      // 1. Global admin → always allowed
      // 2. Project role check (domain's project membership)
      //    release_manager/admin → domain admin
      //    operator → domain editor
      //    viewer → domain viewer
      // 3. Direct domain_permissions check
      // Return highest permission found
  }
  ```

- API endpoints:
  - `GET /api/v1/domains/:id/permissions` — list who has access
  - `POST /api/v1/domains/:id/permissions` — grant `{ "user_id": 5, "permission": "editor" }`
  - `DELETE /api/v1/domains/:id/permissions/:user_id` — revoke
  - Only domain admin (or global admin) can manage permissions

- Middleware integration:
  - New middleware: `DNSPermissionCheck(minLevel)` wraps DNS endpoints
  - Applied to routes:
    - GET dns records → `viewer`
    - POST/PUT/DELETE dns records → `editor`
    - POST dns/plan → `editor`
    - POST dns/apply → `admin`

- Frontend:
  - Domain detail → "Permissions" tab (only visible to admins):
    - User list with current permission level
    - Add user (user selector + permission dropdown)
    - Change permission level
    - Revoke access
  - DNS tab: action buttons disabled/hidden based on user's permission level

**Scope (out)**:

- Team/group-based permissions (only individual users)
- Permission inheritance across domains (each domain is independent)
- API key permissions (use global role for API keys)
- Temporal permissions (time-limited access)

**Deliverables**:

- Permission service with grant/revoke/check
- Two-path access resolution (global role + domain-level)
- API endpoints for permission management
- Middleware for DNS routes
- Frontend permissions tab + UI gating
- `go test ./internal/domain/...` passes

**Acceptance**:

- Global admin can access all domains' DNS without explicit permission
- Project release_manager → has domain admin permission on project's domains
- User with no project role + domain permission "editor" → can edit records
- User with domain permission "viewer" → can see records, cannot edit
- User with domain permission "editor" → can edit, can plan, cannot apply
- Grant permission → visible in permissions list
- Revoke → user loses access immediately
- Non-admin tries to grant permission → 403
- `go test ./internal/domain/...` + middleware tests pass

---

### PB.7 — DNS Templates + Drift Detection

**Owner**: Sonnet
**Depends on**: PB.5 (DNS UI exists), PB.3 (sync worker exists)
**Reads first**: `docs/analysis/POWERDNS_ADMIN_ANALYSIS.md` §2 "Domain Template"

**Context**: Two remaining features: (1) DNS templates pre-populate records
when a domain is first set up for DNS management, and (2) drift detection
alerts operators when someone changes DNS outside the platform.

**Scope (in)**:

- DNS record templates:
  - `api/handler/dnstemplate.go`:
    - `POST /api/v1/dns-templates` — create template
    - `GET /api/v1/dns-templates` — list templates
    - `GET /api/v1/dns-templates/:id` — get detail
    - `PUT /api/v1/dns-templates/:id` — update
    - `DELETE /api/v1/dns-templates/:id` — delete
    - `POST /api/v1/domains/:id/dns/apply-template` — apply template to domain
      `{ "template_id": 3, "variables": {"ip": "1.2.3.4", "mx": "mail.example.com"} }`
  - Template variable substitution: `{{ip}}` in record content → replaced with provided value
  - Applying template = bulk-insert records into `dns_records` (desired state)
  - Does NOT auto-apply to provider — user must still plan/apply

- Drift detection enhancement:
  - Enhance `dns:sync` worker (from PB.3):
    - Compare provider state vs desired state
    - Classify differences:
      - `drift_added`: record exists at provider, not in desired state, not marked unmanaged
      - `drift_modified`: record exists in both but content/TTL differs
      - `drift_deleted`: record in desired state but missing at provider
    - Store drift report: update `domains.last_drift_at`, record in sync_history
  - Notification on drift detection:
    - If drift found → enqueue `notify:send` task
    - Message: "DNS drift detected for example.com: 2 records differ from desired state"
    - Dedup: same domain → max 1 drift notification per 6 hours

- Frontend:
  - `web/src/views/settings/DNSTemplateList.vue`:
    - List templates with name, description, record count
    - Create/edit template: name + description + records editor (same inline table as DNS tab)
    - Variable definition: list variables used in templates
  - `web/src/views/domains/DomainDNS.vue` — enhancements:
    - "Apply Template" button → select template + fill variables → preview records → confirm
    - Drift indicator: "⚠️ Drift detected at {time}" banner when `last_drift_at` is recent
    - "View Drift" button → shows what differs (similar to plan view)

**Scope (out)**:

- Auto-remediation (auto-apply to fix drift) — Phase C with alert engine
- Template versioning (templates are mutable, like PowerDNS-Admin)
- Template marketplace / sharing between projects
- Scheduled plan/apply (user must trigger manually)

**Deliverables**:

- DNS template CRUD API + apply endpoint
- Variable substitution logic
- Enhanced drift detection in sync worker
- Drift notification dispatch
- Frontend: template management + apply wizard + drift indicator
- `npm run build` + `go test ./...` passes

**Acceptance**:

- Create template "Standard Web" with A(@→{{ip}}), CNAME(www→@), MX(@→{{mx}}) → saved
- Apply template to domain with variables {ip:"1.2.3.4", mx:"mail.example.com"}
  → 3 records created in dns_records table with resolved values
- Missing variable → 400 error listing which variables are required
- Drift detection: manually add TXT record at Cloudflare → next sync run detects it
- Drift report shows: "drift_added: TXT _verify → abc123"
- Drift notification sent to configured channel
- Same drift on next run → no duplicate notification within 6 hours
- Drift banner shown in DNS tab UI
- Template list shows record count per template
- `go test ./internal/dnssync/... ./api/handler/...` passes

---

## Phase B Effort Estimate

| # | Task | Owner | Lo | Hi | Risk | Notes |
|---|---|---|---|---|---|---|
| PB.1 | DNS Record Data Model | **Opus** | 1.0 | 2.0 | 🟢 | Straightforward schema |
| PB.2 | Provider Sync Engine | **Opus** | 2.5 | 4.0 | 🔴 | Core correctness path; Cloudflare API integration |
| PB.3 | Plan/Apply Workflow API | Sonnet | 1.5 | 2.5 | 🟡 | Redis caching + checksum + audit trail |
| PB.4 | Safety Thresholds | **Opus** | 1.0 | 1.5 | 🟡 | Logic is simple but testing must be exhaustive |
| PB.5 | DNS Management UI | Sonnet | 2.5 | 4.0 | 🟡 | Staged-edit pattern + inline validation + plan modal |
| PB.6 | Zone-Level RBAC | Sonnet | 1.0 | 2.0 | 🟢 | Standard permission model |
| PB.7 | Templates + Drift | Sonnet | 1.5 | 2.5 | 🟢 | Variable substitution + drift diff |

**Task sum**: Lo = 11.0 days / Hi = 18.5 days

**Integration friction**: +3–4 days (Cloudflare API edge cases, record type
coverage, safety threshold tuning with real data)

| | Work days | Calendar weeks |
|---|---|---|
| **Optimistic** | 14 days | ~3 weeks |
| **Mid-range** | 19 days | ~4 weeks |
| **Pessimistic** | 22.5 days | ~5 weeks |

### Risk hotspots

1. **PB.2 Provider Sync Engine** 🔴 — First time making real DNS changes via
   API. Cloudflare API has quirks (proxy mode, auto-TTL, locked records).
   Must handle partial failures gracefully (some records applied, others failed).
   Mitigation: start with A/CNAME only, add MX/TXT/SRV incrementally.

2. **PB.5 DNS Management UI** 🟡 — Staged-edit is complex frontend state
   management. Inline validation must cover 8+ record types. Plan modal
   needs clear UX for the force-apply flow.
   Mitigation: ship basic table first, add inline edit incrementally.

### Recommended work order

```
Week 1:  PB.1 (schema) + PB.6 (RBAC, independent)
Week 2:  PB.2 (sync engine — start with GetZoneRecords + ComputePlan)
Week 3:  PB.2 (finish ApplyCorrections) + PB.4 (safety) + PB.3 (plan/apply API)
Week 4:  PB.5 (DNS UI — record table + plan modal)
Week 5:  PB.5 (finish inline edit + validation) + PB.7 (templates + drift)
```

---

## Scope Creep Warnings

| Temptation | Truth |
|---|---|
| "PB.2 should support Route53 and PowerDNS too" | One provider (Cloudflare) is enough. Interface makes adding more trivial. |
| "PB.3 should auto-apply on record save" | NEVER auto-apply. Plan/apply is explicit by design. |
| "PB.4 should learn safe thresholds from history" | Static configurable thresholds are fine. ML is way overkill. |
| "PB.5 needs a rich diff viewer with syntax highlighting" | Monospace color-coded text (green/red/yellow) is sufficient. |
| "PB.5 should support zone file import/export" | Out of scope. Use API/CSV for bulk operations. |
| "PB.6 should support team-based permissions" | Individual user permissions only. Teams are a future abstraction. |
| "PB.7 should auto-fix drift" | NEVER auto-fix. Drift = notification only. Auto-remediation is Phase C. |
| "PB.7 templates should be versioned" | Mutable templates (like PowerDNS-Admin). Versioning adds complexity for no Phase B benefit. |

---

## References

- `docs/ARCHITECTURE_ROADMAP.md` §5 — Phase B overview
- `docs/DOMAIN_ASSET_LAYER_DESIGN.md` — DNS provider table (Phase A foundation)
- `docs/analysis/DNSCONTROL_ANALYSIS.md` — Provider interface, Correction pattern
- `docs/analysis/OCTODNS_ANALYSIS.md` — Plan/apply, safety thresholds, sync orchestration
- `docs/analysis/POWERDNS_ADMIN_ANALYSIS.md` — UI patterns, RBAC, templates
- `docs/FRONTEND_GUIDE.md` — Vue 3 component conventions
- `CLAUDE.md` — Tech stack, coding standards
