# PHASE_A_TASKLIST.md — Domain Asset Layer Work Order

> **Created 2026-04-21.** This document is the authoritative work order for
> Phase A (Domain Asset Layer) of the platform restructuring.
>
> **Pre-requisite**: Phase 1-2 complete. Read CLAUDE.md, ARCHITECTURE.md,
> DOMAIN_ASSET_LAYER_DESIGN.md, and the `docs/analysis/` files before
> starting any PA task.
>
> **Audience**: Claude Code sessions (Opus for schema/migration/core logic,
> Sonnet for CRUD/UI tasks).

---

## Phase A — Definition of Scope

Phase A transforms the platform's `domains` table from a thin deployment
target into a **full asset management layer** — the foundation for all
subsequent phases (DNS operations, monitoring, GFW detection).

### What "Phase A done" looks like (acceptance demo)

```
1. Admin navigates to Registrar page → creates "Namecheap" registrar
   with API type "namecheap" and capabilities JSON
2. Admin creates a registrar account under Namecheap (with encrypted credentials)
3. Admin navigates to DNS Provider page → creates "Cloudflare" with
   provider_type "cloudflare", zone config, and credentials
4. Admin creates a domain → assigns registrar account + DNS provider +
   expiry date + annual cost + tags
5. System auto-extracts TLD from FQDN, auto-calculates cost from fee schedule
6. Dashboard shows: 5 domains expiring within 30 days, 2 SSL certs expiring,
   total annual cost $12,340 across 3 registrars
7. Admin triggers bulk import (CSV with 50 domains) → import job processes,
   dedup, and inserts 47 new domains (3 already exist)
8. Admin views domain detail → sees full asset panel: registrar info,
   DNS provider, nameservers, expiry, transfer status, cost history, tags
9. Daily worker runs → updates expiry_status → fires Telegram alert for
   7-day expirations
10. Admin records a domain transfer: status=pending → later confirms
    completion → registrar_account_id updated automatically
```

### What is OUT of Phase A (do not implement)

| Feature | Phase | Reason |
|---|---|---|
| DNS record management (CRUD for A/CNAME/MX) | Phase B | Requires sync engine |
| DNS plan/apply workflow | Phase B | Depends on provider sync |
| Probe L1/L2/L3 | Phase C | Separate subsystem |
| Alert engine with dedup | Phase C | Phase A only does simple expiry notifications |
| Public status pages | Phase C | Requires probe data |
| GFW detection | Phase D | Requires probe infrastructure |
| Registrar API live sync (auto-pull domains) | Phase A8 (deferred) | Nice-to-have; manual import first |
| Multi-currency conversion/normalization | Future | Store per-record currency, display as-is |

---

## Dependency Graph

```
    PA.1 (Schema + Models — foundation for everything)
       │
       ├──────────────────────────────────────┐
       ▼                                      ▼
    PA.2                                    PA.3
  Registrar +                          Domain Asset
  DNS Provider CRUD                    Extension (API+UI)
       │                                      │
       │         ┌────────────────────────────┼────────────────┐
       ▼         ▼                            ▼                ▼
    PA.5       PA.4                         PA.6             PA.7
  Fee Schedule  SSL Cert                   Tags +           Expiry
  + Cost        Tracking                   Bulk Ops         Dashboard
                                                            + Alerts
                                                              │
                                                              ▼
                                                           PA.8
                                                        Import Queue
```

### Critical path

`PA.1 → PA.2 → PA.3 → PA.7`

### Parallelization rules

- PA.2 and PA.3 can start in parallel after PA.1 (PA.3 needs PA.2's tables
  but can stub the UI selectors)
- PA.4, PA.5, PA.6 can all run in parallel after PA.3
- PA.7 depends on PA.3 (needs domain expiry data) and PA.4 (SSL expiry)
- PA.8 depends on PA.2 (needs registrar_accounts for import source)

---

## Task Cards

---

### PA.1 — Schema + Models + Store Layer **(Opus)**

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: **Opus** — schema design is the foundation; errors here are costly
**Depends on**: Phase 1-2 complete
**Reads first**: `docs/DOMAIN_ASSET_LAYER_DESIGN.md` §3 "Table Definitions",
`docs/analysis/DOMAINMOD_ANALYSIS.md` §6 "Revised Domain Table Design",
`docs/analysis/NOMULUS_ANALYSIS.md` §8 "Proposed Schema Changes"

**Context**: The current `domains` table has ~8 columns. Phase A needs it to
have ~35 columns plus 9 new supporting tables. This task creates the full
schema and Go model layer.

**Scope (in)**:

- Edit `migrations/000001_init.up.sql` (pre-launch window) to add:
  - `registrars` table (with `capabilities` JSONB)
  - `registrar_accounts` table (with `credentials` JSONB)
  - `dns_providers` table (with `config`, `credentials` JSONB)
  - `ssl_certificates` table
  - `domain_costs` table
  - `domain_fee_schedules` table
  - `tags` table + `domain_tags` join table
  - `domain_import_jobs` table
  - Extend `domains` table with all new columns (see DOMAIN_ASSET_LAYER_DESIGN.md §3.2):
    - Asset metadata: `tld`, `registrar_account_id`, `dns_provider_id`
    - Registration: `registration_date`, `expiry_date`, `auto_renew`, `grace_end_date`, `expiry_status`
    - Status flags: `transfer_lock`, `hold`
    - Transfer: `transfer_status`, `transfer_gaining_registrar`, `transfer_requested_at`, `transfer_completed_at`, `last_transfer_at`, `last_renewed_at`
    - DNS: `nameservers` JSONB, `dnssec_enabled`
    - WHOIS: `whois_privacy`, `registrant_contact` JSONB, `admin_contact` JSONB, `tech_contact` JSONB
    - Financial: `annual_cost`, `currency`, `purchase_price`, `fee_fixed`
    - Metadata: `purpose`, `notes`, `metadata` JSONB
  - All necessary indexes (expiry_date, tld, registrar_account, dns_provider)

- Go model structs:
  - `internal/registrar/model.go`: `Registrar`, `RegistrarAccount`
  - `internal/dnsprovider/model.go`: `DNSProvider`
  - `internal/domain/model.go`: extended `Domain` struct (update existing if present)
  - `internal/ssl/model.go`: `SSLCertificate`
  - `internal/cost/model.go`: `DomainCost`, `DomainFeeSchedule`
  - `internal/tag/model.go`: `Tag`
  - `internal/importer/model.go`: `DomainImportJob`

- Repository interfaces:
  - `internal/registrar/repository.go`: `RegistrarStore` interface
  - `internal/dnsprovider/repository.go`: `DNSProviderStore` interface
  - `internal/ssl/repository.go`: `SSLCertStore` interface
  - `internal/cost/repository.go`: `CostStore` interface
  - `internal/tag/repository.go`: `TagStore` interface

- Store implementations (sqlx):
  - `store/postgres/registrar.go`
  - `store/postgres/dnsprovider.go`
  - `store/postgres/ssl.go`
  - `store/postgres/cost.go`
  - `store/postgres/tag.go`
  - Update `store/postgres/domain.go` (extended queries)

**Scope (out)**:

- API handlers (PA.2, PA.3)
- Frontend (PA.2, PA.3)
- Business logic (fee calculation, expiry check) — PA.5, PA.7
- Import logic — PA.8

**Deliverables**:

- Updated migration SQL (all 9 new tables + domains extension)
- Go model structs for all new entities
- Repository interfaces
- Store implementations (CRUD methods)
- `make migrate-up` succeeds without errors
- All existing tests still pass (no regression)

**Acceptance**:

- `make migrate-up && make migrate-down && make migrate-up` — idempotent
- All new tables exist with correct columns, types, constraints, indexes
- `domains` table has all 20+ new columns (nullable where appropriate)
- Go structs have correct `db:""` and `json:""` tags
- Store methods pass basic unit tests (insert + get + list + update)
- Foreign key constraints work (e.g., can't insert domain with invalid registrar_account_id)
- `go build ./...` succeeds
- `go test ./store/postgres/...` passes

**Delivered (2026-04-21)**:

- `migrations/000001_init.up.sql` — added `registrars`, `registrar_accounts`,
  `dns_providers`, `ssl_certificates`, `domain_fee_schedules`, `domain_costs`,
  `tags`, `domain_tags`, `domain_import_jobs` tables; extended `domains` with
  25+ asset columns (TLD, registrar binding, expiry, transfer tracking, DNS,
  contacts, financial, metadata); added CHECK constraints + indexes
- `migrations/000001_init.down.sql` — updated with reverse drops
- `store/postgres/domain.go` — `Domain` struct extended with all new fields;
  `domainColumns` constant for DRY queries; added `UpdateAssetFields()`,
  `UpdateExpiryStatus()`, `UpdateTransferStatus()`, `ListExpiring()`
- `store/postgres/registrar.go` — `Registrar`, `RegistrarAccount` structs;
  `RegistrarStore` with full CRUD + dependency checks on delete
- `store/postgres/dns_provider.go` — `DNSProvider` struct; `DNSProviderStore`
  with CRUD + dependency checks
- `store/postgres/ssl_certificate.go` — `SSLCertificate` struct;
  `SSLCertificateStore` with CRUD + `Upsert()` + `ListExpiring()`
- `store/postgres/cost.go` — `DomainFeeSchedule`, `DomainCost` structs;
  `CostStore` with fee schedule CRUD + cost CRUD + summary queries
- `store/postgres/tag.go` — `Tag` struct; `TagStore` with CRUD +
  `SetDomainTags()` + `GetDomainTags()` + `ListWithCounts()`
- `internal/lifecycle/service.go` — updated `RegisterInput` (removed old
  `DNSProvider`/`DNSZone`, added `DNSProviderID`)
- `api/handler/domain.go` — updated request/response for new domain fields
- `docs/DATABASE_SCHEMA.md` — added Phase A table index
- `go build ./...` passes; `go vet` passes on all changed packages

---

### PA.2 — Registrar + DNS Provider CRUD (API + UI)

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: Sonnet
**Depends on**: PA.1 (tables and store layer must exist)
**Reads first**: `docs/DOMAIN_ASSET_LAYER_DESIGN.md` §6 "API Endpoints",
`docs/FRONTEND_GUIDE.md`, `docs/analysis/DOMAINMOD_ANALYSIS.md` §3 "Key Design
Patterns"

**Context**: Registrars and DNS providers are reference data that domains link
to. They must be manageable before domains can be assigned to them.

**Scope (in)**:

- `internal/registrar/service.go`:
  - `Create(ctx, input) (*Registrar, error)`
  - `Get(ctx, id) (*Registrar, error)`
  - `List(ctx, filter) ([]Registrar, error)`
  - `Update(ctx, id, input) (*Registrar, error)`
  - `Delete(ctx, id) error` (soft delete)
  - Same CRUD for `RegistrarAccount`

- `internal/dnsprovider/service.go`:
  - Same CRUD pattern
  - Validate `provider_type` against `pkg/provider/dns` registry

- `api/handler/registrar.go`:
  - `POST /api/v1/registrars` — create
  - `GET /api/v1/registrars` — list (with search, pagination)
  - `GET /api/v1/registrars/:id` — get detail (includes accounts + domain count)
  - `PUT /api/v1/registrars/:id` — update
  - `DELETE /api/v1/registrars/:id` — soft delete
  - `POST /api/v1/registrars/:id/accounts` — create account
  - `GET /api/v1/registrars/:id/accounts` — list accounts
  - `PUT /api/v1/registrar-accounts/:id` — update account
  - `DELETE /api/v1/registrar-accounts/:id` — soft delete

- `api/handler/dnsprovider.go`:
  - `POST /api/v1/dns-providers` — create
  - `GET /api/v1/dns-providers` — list
  - `GET /api/v1/dns-providers/:id` — get detail (includes domain count)
  - `PUT /api/v1/dns-providers/:id` — update
  - `DELETE /api/v1/dns-providers/:id` — soft delete

- `api/router/router.go` — register new routes (admin role required)

- `cmd/server/main.go` — wire new services and handlers

- Frontend:
  - `web/src/types/registrar.ts` — TypeScript types
  - `web/src/types/dnsprovider.ts`
  - `web/src/api/registrar.ts` — API client
  - `web/src/api/dnsprovider.ts`
  - `web/src/stores/registrar.ts` — Pinia store
  - `web/src/stores/dnsprovider.ts`
  - `web/src/views/registrars/RegistrarList.vue` — list + create form
  - `web/src/views/registrars/RegistrarDetail.vue` — edit + accounts list
  - `web/src/views/dns-providers/DNSProviderList.vue` — list + create form
  - `web/src/views/dns-providers/DNSProviderDetail.vue` — edit
  - Router routes + sidebar entries

**Scope (out)**:

- Registrar API integration (auto-sync) — PA.8
- DNS record management — Phase B
- Credential encryption implementation (store as JSONB; encryption is a
  cross-cutting concern handled separately)

**Deliverables**:

- Service layer for registrars + DNS providers
- API handlers + routes
- Frontend pages (list + detail + create/edit)
- `npm run build` clean
- `go test ./internal/registrar/... ./internal/dnsprovider/...` passes

**Acceptance**:

- Create registrar "Namecheap" with api_type "namecheap" → visible in list
- Create account under Namecheap with credentials JSONB → visible in detail
- Create DNS provider "Cloudflare" with provider_type "cloudflare" → visible
- Edit registrar capabilities → saved correctly
- Delete registrar with domains attached → returns 409 (has dependencies)
- Delete registrar with no domains → soft deletes (deleted_at set)
- List endpoints support pagination (`?page=1&per_page=20`)
- Frontend forms validate required fields
- `npm run build` succeeds with zero TypeScript errors

**Delivered (2026-04-21)**:

- `internal/registrar/service.go` — `Service` wrapping `RegistrarStore`:
  `Create`, `GetByID`, `List`, `Update`, `Delete`, `CreateAccount`,
  `GetAccount`, `ListAccounts`, `UpdateAccount`, `DeleteAccount`;
  validates name non-empty; defaults capabilities/credentials to `{}`;
  maps store sentinel errors to package-level sentinels
- `internal/registrar/service_test.go` — 4 unit tests (name validation,
  capability default, error sentinel distinctness)
- `internal/dnsprovider/service.go` — `Service` wrapping `DNSProviderStore`:
  full CRUD + `SupportedTypes()` helper; validates `provider_type` against
  `KnownProviderTypes` map (cloudflare, route53, dnspod, alidns, godaddy,
  namecheap, manual)
- `internal/dnsprovider/service_test.go` — 7 unit tests (known/unknown types,
  name validation, config default, sentinel distinctness, SupportedTypes)
- `api/handler/registrar.go` — handlers for all registrar + account endpoints;
  credentials intentionally excluded from responses (security)
- `api/handler/dnsprovider.go` — handlers for all DNS provider endpoints;
  `SupportedTypes` endpoint returns dropdown values for frontend
- `api/router/router.go` — registered `/registrars`, `/registrar-accounts`,
  `/dns-providers` route groups with RBAC (admin for write, viewer+ for read)
- `cmd/server/main.go` — wired `RegistrarStore → Service → Handler` and
  `DNSProviderStore → Service → Handler`
- `web/src/types/registrar.ts` — `RegistrarResponse`, `RegistrarAccountResponse`,
  create/update request types
- `web/src/types/dnsprovider.ts` — `DNSProviderResponse`, `DNSProviderType`
  union, create/update request types
- `web/src/api/registrar.ts` — full API client for registrars + accounts
- `web/src/api/dnsprovider.ts` — API client for DNS providers
- `web/src/stores/registrar.ts` — Pinia store with all CRUD actions
- `web/src/stores/dnsprovider.ts` — Pinia store with all CRUD actions +
  `fetchTypes()` for dropdown
- `web/src/views/registrars/RegistrarList.vue` — list table + create modal
- `web/src/views/registrars/RegistrarDetail.vue` — detail + edit modal +
  accounts table + create account modal
- `web/src/views/dns-providers/DNSProviderList.vue` — list + create modal with
  dynamic type dropdown
- `web/src/views/dns-providers/DNSProviderDetail.vue` — detail + edit modal
- `web/src/router/index.ts` — 4 new routes added (RegistrarList, RegistrarDetail,
  DNSProviderList, DNSProviderDetail)
- `web/src/views/layouts/MainLayout.vue` — "資產管理" nav group added with
  Registrar + DNS Provider sidebar entries
- `go build ./...` passes; `go test ./internal/registrar/... ./internal/dnsprovider/...`
  all 11 tests pass; `npm run build` zero TypeScript errors

---

### PA.3 — Domain Asset Extension (API + UI)

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: Sonnet
**Depends on**: PA.1 (extended domain model), PA.2 (registrar/provider selectors)
**Reads first**: `docs/DOMAIN_ASSET_LAYER_DESIGN.md` §3.2 "domains table",
`docs/analysis/NOMULUS_ANALYSIS.md` §7 "What We Should Adopt",
`docs/analysis/DOMAINMOD_ANALYSIS.md` §5 "What to Adopt"

**Context**: The domain create/edit/list/detail views currently show only FQDN +
lifecycle_state + project. This task extends them to show and edit the full
asset data.

**Scope (in)**:

- Update `internal/domain/service.go` (or create if doesn't exist):
  - Extend `Create()` input to accept asset fields
  - Extend `Update()` to allow editing asset fields
  - Add `tld` auto-extraction: parse FQDN → extract TLD on create/update
  - Add `annual_cost` auto-calculation from fee schedule (unless fee_fixed)
  - New method: `GetAssetDetail(ctx, id)` — returns domain + registrar + provider + certs + costs
  - New method: `ListExpiring(ctx, days int)` — domains expiring within N days
  - New method: `GetStats(ctx, projectID)` — aggregate stats

- Update `api/handler/domain.go`:
  - Extend `POST /api/v1/domains` request/response with asset fields
  - Extend `PUT /api/v1/domains/:id` with asset fields
  - Extend `GET /api/v1/domains/:id` response to include full asset data
  - Extend `GET /api/v1/domains` with new filters:
    - `?registrar_id=`, `?dns_provider_id=`, `?tld=`, `?tag=`
    - `?expiring_within_days=`, `?expiry_status=`
  - New: `GET /api/v1/domains/expiring?days=30` — expiring domains
  - New: `GET /api/v1/domains/stats` — counts by registrar, TLD, provider

- Update `api/handler/domain.go` — Transfer tracking:
  - `POST /api/v1/domains/:id/transfer` — record transfer initiation
    `{ "gaining_registrar_account_id": 5, "notes": "..." }`
  - `POST /api/v1/domains/:id/transfer/complete` — confirm completion
  - `POST /api/v1/domains/:id/transfer/cancel` — cancel

- Frontend:
  - Update `web/src/types/domain.ts` — extend DomainResponse with all asset fields
  - Update `web/src/views/domains/DomainList.vue`:
    - New columns: registrar, DNS provider, expiry_date, annual_cost, tags
    - New filters: registrar, provider, TLD, expiry range
    - Sort by expiry (soonest first default option)
  - Update `web/src/views/domains/DomainDetail.vue`:
    - New "Asset" tab: registrar info, DNS provider, nameservers, DNSSEC
    - New "Registration" section: dates, auto_renew, transfer_lock, privacy
    - New "Transfer" section: status, gaining registrar, dates
    - New "Financial" section: annual_cost, currency, fee_fixed indicator
    - New "Contacts" section: registrant, admin, tech (collapsible JSONB display)
  - Update domain create/edit form:
    - Registrar account selector (grouped by registrar)
    - DNS provider selector
    - Expiry date picker
    - Auto-renew toggle
    - Cost fields (with fee_fixed override toggle)
    - Purpose field
    - Notes field

**Scope (out)**:

- Tag assignment UI (PA.6)
- Cost history (PA.5)
- SSL cert display (PA.4)
- Bulk operations (PA.6)
- Import (PA.8)

**Deliverables**:

- Extended domain service with asset logic
- Extended domain API (create/update/get/list with new fields + filters)
- Transfer tracking endpoints
- Extended frontend (list columns, detail tabs, create/edit form)
- TLD auto-extraction working
- Cost auto-calculation working

**Acceptance**:

- Create domain with registrar_account + dns_provider + expiry → all saved
- `tld` auto-extracted correctly (".com" from "example.com", ".co.uk" from "test.co.uk")
- Domain list shows registrar + provider + expiry columns
- Filter by registrar → shows only domains at that registrar
- Domain detail "Asset" tab shows full information
- Transfer flow: initiate → status="pending" → complete → registrar_account updated
- `GET /api/v1/domains/expiring?days=30` returns correct domains
- `GET /api/v1/domains/stats` returns counts grouped by registrar, TLD
- `npm run build` + `go test ./...` passes

**Delivered (2026-04-21)**:

- `store/postgres/domain.go` — Added `ListFilter` struct with optional pointer
  fields; `ListWithFilter()` + `CountWithFilter()` using dynamic positional-param
  WHERE clause; `DomainStats` + `GetStats()` aggregate query
- `internal/lifecycle/errors.go` — Added `ErrTransferAlreadyPending`,
  `ErrNoActiveTransfer` sentinel errors
- `internal/lifecycle/service.go` — Complete rewrite:
  - `ExtractTLD(fqdn)` — ccSLD heuristic (handles `.co.uk`, `.com.au`, etc.)
  - Extended `RegisterInput` with all asset fields (RegistrarAccountID,
    DNSProviderID, ExpiryDate, AutoRenew, AnnualCost, Currency, Purpose, Notes)
  - `Register()` now auto-extracts and stores TLD from FQDN
  - `ListInput` with `*int64` optional filters (ProjectID, RegistrarID,
    DNSProviderID) + TLD/LifecycleState/ExpiryStatus string filters + cursor/limit
  - `UpdateAssetInput` + `UpdateAsset()` — updates all non-identity asset fields
  - `InitiateTransfer()`, `CompleteTransfer()`, `CancelTransfer()` — transfer
    state machine with sentinel error guards
  - `ListExpiring(days)`, `GetStats(projectID)` — aggregate queries
- `internal/lifecycle/tld_test.go` — 29 TLD extraction test cases covering simple
  TLDs, ccSLDs (`.co.uk`, `.com.au`, `.org.uk`), uppercase normalization, single-
  label FQDNs; plus sentinel error distinctness tests; all 29 pass
- `api/handler/domain.go` — Complete rewrite:
  - Extended `RegisterDomainRequest` + `UpdateDomainAssetRequest` request DTOs
  - `domainResponse()` — now returns 30+ fields (identity, provider binding,
    dates, status flags, transfer tracking, DNS, WHOIS, financial, metadata)
  - `Register()`, `Get()`, `List()`, `UpdateAsset()`, `Transition()` handlers
  - `Expiring()`, `Stats()` handlers
  - `InitiateTransfer()`, `CompleteTransfer()`, `CancelTransfer()` handlers
- `api/router/router.go` — Added domain routes in correct order:
  `GET /expiring`, `GET /stats` registered before `/:id`; added `PUT /:id`,
  `POST /:id/transfer`, `POST /:id/transfer/complete`, `POST /:id/transfer/cancel`
- `web/src/types/domain.ts` — Full rewrite with `DomainResponse` (30+ fields),
  `RegisterDomainRequest`, `UpdateDomainAssetRequest`, `InitiateTransferRequest`,
  `DomainStats`, `DomainLifecycleHistoryEntry`, `TransferStatus`, `ExpiryStatus`
- `web/src/api/domain.ts` — Full rewrite with all endpoints: list, get, register,
  updateAsset, transition, history, expiring, stats, initiateTransfer,
  completeTransfer, cancelTransfer
- `web/src/stores/domain.ts` — Full rewrite with all store actions matching new API
- `web/src/views/domains/DomainList.vue` — Full rewrite:
  - New columns: TLD, expiry_date (color-coded — red ≤7d, orange ≤30d), auto_renew,
    annual_cost with currency
  - Filter bar: lifecycle state, registrar, DNS provider, TLD input, expiry status
  - Extended create form with registrar+account (cascading selectors), DNS provider,
    NDatePicker for expiry, NSwitch for auto-renew, cost fields (amount + currency)
- `web/src/views/domains/DomainDetail.vue` — Full rewrite with 3-tab layout:
  - "資產" tab: registration info (NDescriptions), financial info, DNS+security flags
  - "轉移" tab: transfer status display + initiate/complete/cancel flow with modals
  - "歷史" tab: lifecycle state history timeline (preserved from original)
  - Edit asset modal: full form with all updatable fields
- `go build ./...` passes; `go test ./internal/lifecycle/...` 29 tests pass;
  `npm run build` zero TypeScript errors, zero warnings

---

### PA.4 — SSL Certificate Tracking

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: Sonnet
**Depends on**: PA.1 (ssl_certificates table), PA.3 (domain detail page exists)
**Reads first**: `docs/DOMAIN_ASSET_LAYER_DESIGN.md` §3.2 "ssl_certificates",
`docs/analysis/DOMAINMOD_ANALYSIS.md` §2 "ssl_certs table"

**Context**: SSL cert expiry is as critical as domain expiry — an expired cert
= site down. This task adds cert tracking (metadata only) and automated
expiry checking via TLS connection probing.

**Scope (in)**:

- `internal/ssl/service.go`:
  - `Create(ctx, input) (*SSLCertificate, error)` — manual add
  - `Get(ctx, id)`, `List(ctx, domainID)`, `Delete(ctx, id)`
  - `CheckExpiry(ctx, domainID) (*SSLCertificate, error)` — connect to domain:443,
    extract cert info (issuer, serial, expiry, subject), upsert into table
  - `CheckAllExpiring(ctx) error` — batch check all active domains

- asynq task: `ssl:check_expiry` (runs daily):
  - For each active domain: TLS connect → extract cert → upsert
  - Detect state changes: `active → expiring → expired`
  - On state change → enqueue `notify:send` task

- `api/handler/ssl.go`:
  - `POST /api/v1/domains/:id/ssl-certs` — manual add
  - `GET /api/v1/domains/:id/ssl-certs` — list certs for domain
  - `GET /api/v1/ssl-certs/expiring?days=30` — all expiring certs
  - `POST /api/v1/domains/:id/ssl-certs/check` — trigger manual check

- Frontend:
  - `web/src/views/domains/DomainDetail.vue` — "SSL" tab:
    - Current cert info (issuer, expiry, status badge)
    - "Check Now" button
    - Cert history (if multiple certs over time)
  - Expiry dashboard widget (merged with domain expiry in PA.7)

**Scope (out)**:

- Cert issuance/renewal (ACME, Let's Encrypt integration)
- Cert content storage (private keys, CSRs)
- Multi-SAN cert tracking (one cert → multiple domains)
- Certificate chain validation

**Deliverables**:

- SSL service with TLS connection checker
- asynq worker handler for periodic checking
- API endpoints
- Frontend SSL tab in domain detail
- Cert expiry detection + notification trigger

**Acceptance**:

- `POST /api/v1/domains/:id/ssl-certs/check` on a live HTTPS domain → cert info saved
- `ssl_certificates` row has correct: issuer, expires_at, serial_number, status
- Daily worker checks all active domains → updates cert status
- Cert expiring within 30 days → status = "expiring"
- Cert expired → status = "expired"
- State change (active→expiring) triggers notification task
- `GET /api/v1/ssl-certs/expiring?days=7` returns correct list
- `go test ./internal/ssl/...` passes

**Delivered (2026-04-21)**:

- `internal/tasks/types.go` — Added `TypeSSLCheckExpiry` ("ssl:check_expiry") and
  `TypeSSLCheckAllActive` ("ssl:check_all_active") task type constants
- `internal/ssl/service.go` — `Service` with:
  - `ComputeSSLStatus(expiresAt)` — pure function returning "active"/"expiring"/"expired";
    threshold is 30 days; testable via injected `computeSSLStatusAt(expiresAt, now)`
  - `Create()` — manual cert add with auto-computed status
  - `GetByID()`, `List(domainID)`, `ListExpiring(days)`, `Delete()`
  - `CheckExpiry(ctx, domainID, fqdn)` — dials `fqdn:443` with `tls.Dialer`, validates
    cert chain using system trust store, extracts leaf cert issuer/serial/dates, upserts
    via `SSLCertificateStore.Upsert()`; connection uses context deadline for timeout
  - `CheckAllActive(ctx)` — lists all `lifecycle_state='active'` domains via
    `DomainStore.ListWithFilter`, runs `CheckExpiry` for each, returns (checked, failed) counts
- `internal/ssl/task.go` — asynq handlers:
  - `HandleCheckExpiry` + `NewCheckExpiryTask()` — single-domain TLS probe; returns nil
    on TLS failure (unreachable hosts should not trigger retries)
  - `HandleCheckAllActive` — batch handler; logs final checked/failed counts
- `internal/ssl/service_test.go` — 12 unit tests:
  - `TestComputeSSLStatus` — 10 boundary cases (90d active, 31d active, 30d expiring,
    7d expiring, 1s expiring, now expired, 1s ago expired, 30d ago expired, 1y ago expired)
  - `TestStatusConstants` — all 4 status strings are distinct and non-empty
  - `TestExpiryThreshold` — exact boundary at 30d and 30d+1ns
- `api/handler/ssl.go` — 5 handlers: `Create`, `List`, `Check`, `ListExpiring`, `Delete`
  with correct HTTP status codes (201/200/200/200/200); `DaysLeft` field computed on
  response; `ErrCheckFailed` maps to 502 Bad Gateway
- `api/router/router.go` — registered SSL routes:
  - `GET /ssl-certs/expiring` (static path before /:id)
  - `DELETE /ssl-certs/:id`
  - `POST /domains/:id/ssl-certs`, `GET /domains/:id/ssl-certs`,
    `POST /domains/:id/ssl-certs/check`
  - `SSLHandler` field added to `Deps` struct
- `cmd/server/main.go` — wired `SSLCertificateStore → ssl.Service → SSLHandler`
- `cmd/worker/main.go` — registered `HandleCheckExpiry` and `HandleCheckAllActive`
  as real asynq handlers (replacing prior stubs)
- `web/src/types/ssl.ts` — `SSLCertResponse`, `CreateSSLCertRequest`, `SSLCheckRequest`,
  `SSLStatus` union type
- `web/src/api/ssl.ts` — API client for all 5 endpoints
- `web/src/stores/ssl.ts` — Pinia store with `fetchList`, `create`, `check` (with
  local upsert into certs array), `fetchExpiring`, `deleteCert`
- `web/src/views/domains/DomainDetail.vue` — added SSL tab:
  - NDataTable with columns: expires_at, days_left (color-coded), status (NTag),
    issuer, serial_number, last_check_at, delete action (NPopconfirm)
  - "立即檢查" button → calls `sslStore.check(domainId, fqdn)` live TLS probe
  - "手動新增" button → modal with expires_at (required), issuer, cert_type (select),
    serial_number, notes
- `go build ./...` passes; `go test ./internal/ssl/...` 12 tests pass;
  `npm run build` zero TypeScript errors

---

### PA.5 — Fee Schedule + Cost Tracking

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: Sonnet
**Depends on**: PA.1 (domain_fee_schedules + domain_costs tables), PA.2
(registrar exists), PA.3 (domain has annual_cost field)
**Reads first**: `docs/analysis/DOMAINMOD_ANALYSIS.md` §7 "Fee Schedule Design",
`docs/DOMAIN_ASSET_LAYER_DESIGN.md` D9

**Context**: DomainMOD's fee model is per (registrar × TLD). We adopt this:
a fee schedule defines standard pricing, and domains auto-inherit unless
`fee_fixed = true`. Cost history tracks actual payments.

**Scope (in)**:

- `internal/cost/service.go`:
  - Fee schedules: CRUD for `domain_fee_schedules`
  - Cost records: CRUD for `domain_costs` (per-event: registration, renewal, transfer)
  - `CalculateAnnualCost(ctx, domainID)` — lookup fee schedule by
    (domain's registrar_id, domain's TLD) → return renewal_fee
  - `RecalculateAllCosts(ctx)` — batch update annual_cost for all non-fixed domains
  - `GetCostSummary(ctx, filter)` — aggregate: total by registrar, TLD, project, period

- `api/handler/cost.go`:
  - `POST /api/v1/fee-schedules` — create fee schedule entry
  - `GET /api/v1/fee-schedules` — list (filter by registrar, TLD)
  - `PUT /api/v1/fee-schedules/:id` — update
  - `DELETE /api/v1/fee-schedules/:id` — delete
  - `POST /api/v1/domains/:id/costs` — add cost record
  - `GET /api/v1/domains/:id/costs` — list cost history for domain
  - `GET /api/v1/costs/summary` — aggregate report
    `?group_by=registrar|tld|project&period=2026`

- Auto-calculation hook:
  - When domain is created/updated with registrar_account_id or TLD change:
    if `fee_fixed = false`, recalculate `annual_cost` from fee schedule
  - When fee schedule is updated: recalculate all affected domains

- Frontend:
  - `web/src/views/settings/FeeScheduleList.vue` — manage fee schedules
    (table: registrar, TLD, registration/renewal/transfer/privacy fee, currency)
  - `web/src/views/domains/DomainDetail.vue` — "Cost" tab:
    - Annual cost display (with "auto" or "manual" indicator)
    - Cost history table (date, type, amount)
    - Add cost record form
  - Dashboard widget: total annual cost by registrar (pie chart or bar)

**Scope (out)**:

- Multi-currency conversion (display in original currency)
- Invoice generation
- Payment tracking / accounts payable integration
- Cost projection / forecasting

**Deliverables**:

- Cost service (fee schedules + cost records + auto-calculation)
- API endpoints for fee schedules and cost records
- Auto-calculation on domain create/update
- Frontend: fee schedule management + domain cost tab + dashboard widget
- Cost summary API

**Acceptance**:

- Create fee schedule: Namecheap × .com = $10.98 renewal → saved
- Create domain at Namecheap with TLD .com → `annual_cost` auto-set to $10.98
- Domain with `fee_fixed = true` → annual_cost NOT overwritten
- Update fee schedule → affected domains' annual_cost recalculated
- Add cost record (type=renewal, amount=$10.98) → visible in history
- `GET /api/v1/costs/summary?group_by=registrar` → correct totals
- `go test ./internal/cost/...` passes

**Delivered (2026-04-21)**:

- `store/postgres/domain.go` — Added `UpdateAnnualCost(ctx, domainID, cost, currency)` — only
  updates rows where `fee_fixed = false` (WHERE clause guard prevents overwriting operator-fixed prices)
- `internal/cost/service.go` — `Service` wrapping `CostStore + DomainStore + RegistrarStore`:
  - `ValidCostTypes` map (registration/renewal/transfer/privacy/other)
  - `ValidCurrencies` map (USD/EUR/GBP/CNY/TWD/JPY/AUD/CAD/HKD/SGD/KRW/INR)
  - Fee schedule CRUD: `CreateFeeSchedule` (with TLD normalization + duplicate detection),
    `GetFeeScheduleByID`, `ListFeeSchedules` (optional registrar filter), `UpdateFeeSchedule`,
    `DeleteFeeSchedule`
  - Cost records: `CreateCost` (validates cost_type + currency), `ListCostsByDomain`
  - `TryApplyFeeSchedule(ctx, domainID)` — load domain → get registrar_account → get registrar →
    lookup fee schedule by (registrar_id, tld) → call `UpdateAnnualCost`; returns nil on any
    missing step (never fails a domain operation because of missing pricing data)
  - `RecalculateAllCosts(ctx)` — batch TryApplyFeeSchedule across all non-fee-fixed domains,
    returns (updated, skipped, failed) counts
  - `GetCostSummary(ctx, groupBy)` — delegates to store aggregate queries (by registrar or tld)
  - `normalizeTLD` — prepends "." and lowercases; idempotent
  - `validateCurrency` — case-insensitive lookup against ValidCurrencies map
- `internal/cost/service_test.go` — 22 assertions across 5 tests:
  - `TestNormalizeTLD` — 9 cases (.com, com, .co.uk, trim, empty)
  - `TestValidateCurrency` — 8 valid + 4 invalid (ErrInvalidCurrency wrapping verified)
  - `TestValidCostTypes` — 5 valid + 4 invalid
  - `TestSentinelErrors` — 4 sentinel errors are distinct non-empty strings
  - `TestNormalizeTLDIdempotent` — 6 inputs verified idempotent
- `api/handler/cost.go` — 6 handlers: `CreateFeeSchedule` (201), `ListFeeSchedules` (200),
  `UpdateFeeSchedule` (200/404), `DeleteFeeSchedule` (200/404), `CreateDomainCost` (201),
  `ListDomainCosts` (200), `GetCostSummary` (200, group_by=registrar|tld)
- `api/router/router.go` — registered routes:
  - `POST/GET /fee-schedules`, `PUT/DELETE /fee-schedules/:id`
  - `POST/GET /domains/:id/costs`
  - `GET /costs/summary?group_by=registrar|tld`
  - `CostHandler` added to `Deps` struct
- `cmd/server/main.go` — wired `CostStore → cost.Service → CostHandler`
- `web/src/types/cost.ts` — `FeeScheduleResponse`, `DomainCostResponse`, `CostSummaryItem`,
  `CostType` union, create/update request types
- `web/src/api/cost.ts` — full API client for all 7 endpoints
- `web/src/stores/cost.ts` — Pinia store with all actions + local array updates on create/delete
- `web/src/views/settings/FeeScheduleList.vue` — list table (registrar name lookup, per-type fee
  columns), filter by registrar, create modal, edit modal, NPopconfirm delete
- `web/src/views/domains/DomainDetail.vue` — added "費用記錄" tab with NDataTable (cost_type,
  amount+currency, paid_at, period dates, notes) + "新增費用記錄" modal
- `web/src/router/index.ts` — added `/settings/fee-schedules` route
- `web/src/views/layouts/MainLayout.vue` — added "費率表管理" sidebar entry
- `go build ./...` passes; `go test ./internal/cost/...` 5 tests pass;
  `npm run build` zero TypeScript errors

---

### PA.6 — Tags + Bulk Operations

**Status**: ✅ COMPLETED 2026-04-21

**Owner**: Sonnet (executed by Opus)
**Depends on**: PA.1 (tags + domain_tags tables), PA.3 (domain list page)
**Reads first**: `docs/DOMAIN_ASSET_LAYER_DESIGN.md` D8

**Context**: Tags replace DomainMOD's rigid single-category model. Domains can
have multiple tags (production, asia, gambling, core). Bulk operations allow
mass-updating domain properties.

**Scope (in)**:

- `internal/tag/service.go`:
  - Tags CRUD: Create, List, Update, Delete
  - `SetDomainTags(ctx, domainID, tagIDs []int64)` — replace domain's tags
  - `GetDomainsByTag(ctx, tagID) ([]Domain, error)`
  - `GetTagsForDomain(ctx, domainID) ([]Tag, error)`

- `api/handler/tag.go`:
  - `POST /api/v1/tags` — create tag (name + color)
  - `GET /api/v1/tags` — list all tags (with domain count per tag)
  - `PUT /api/v1/tags/:id` — update (name, color)
  - `DELETE /api/v1/tags/:id` — delete (detach from all domains first)
  - `PUT /api/v1/domains/:id/tags` — set domain tags `{ "tag_ids": [1,3,5] }`
  - `GET /api/v1/domains?tag=production` — filter by tag name

- Bulk operations API:
  - `POST /api/v1/domains/bulk` — bulk update
    ```json
    {
      "domain_ids": [1, 2, 3, 5, 8],
      "action": "update",
      "fields": {
        "registrar_account_id": 3,
        "dns_provider_id": 2,
        "auto_renew": true
      }
    }
    ```
  - `POST /api/v1/domains/bulk` — bulk tag
    ```json
    {
      "domain_ids": [1, 2, 3],
      "action": "add_tags",
      "tag_ids": [5, 7]
    }
    ```
  - `POST /api/v1/domains/export` — CSV export (filtered)

- Frontend:
  - `web/src/views/settings/TagList.vue` — tag CRUD with color picker
  - Domain list: tag filter dropdown + tag badges on each row
  - Domain detail: tag editor (multi-select with color chips)
  - Bulk action bar: appears when domains are selected (checkbox column)
    - "Assign Registrar", "Assign Provider", "Add Tags", "Set Auto-Renew"
  - CSV export button on domain list

**Scope (out)**:

- Saved filters / segments (frontend-only feature, future)
- Tag-based automation rules (e.g., "all 'production' domains get L3 probe")
- Tag hierarchy / nested tags

**Deliverables**:

- Tag service + API
- Bulk update API (update fields, add/remove tags)
- CSV export
- Frontend: tag manager, tag filter, tag editor, bulk action bar
- `npm run build` + `go test ./...` passes

**Acceptance**:

- Create tag "production" with color #28a745 → visible in list
- Assign tags to domain → visible in domain detail + domain list row
- Filter domain list by tag → correct subset
- Bulk select 5 domains → "Assign Registrar" → all 5 updated
- Bulk add tag → all selected domains gain the tag
- CSV export includes all visible columns + tags
- Delete tag → detaches from all domains, tag removed
- `go test ./internal/tag/...` passes

**Delivered (2026-04-21)**:

- `store/postgres/domain.go` — Added `TagID *int64` field to `ListFilter` (subquery
  join against `domain_tags`); `BulkUpdateFields(ctx, ids, registrarAccountID,
  dnsProviderID, autoRenew)` with dynamic SET clause and positional $N params
- `internal/lifecycle/service.go` — Added `TagID *int64` to `ListInput`, propagated
  to `ListFilter`
- `api/handler/domain.go` — Added `tag_id` query param parsing to `List` handler
- `web/src/api/domain.ts` — Added `tag_id` to `DomainListParams`
- `internal/tag/service.go` — `Service` wrapping `TagStore + DomainStore`:
  - `ValidateColor(*string)` — `#RRGGBB` hex regex; nil accepted
  - Tag CRUD: `Create` (with duplicate name detection), `GetByID`, `ListWithCounts`,
    `Update`, `Delete` (CASCADE handles domain_tags)
  - `GetDomainTags`, `SetDomainTags` — per-domain tag read/write
  - `BulkAddTags(domainIDs, tagIDs)` — union merge (reads existing, adds new)
  - `BulkRemoveTags(domainIDs, tagIDs)` — set difference
  - `BulkUpdateFields` — delegates to `DomainStore.BulkUpdateFields`
  - `ExportDomains` — delegates to `ListWithFilter` for CSV export
- `internal/tag/service_test.go` — 3 tests:
  - `TestValidateColor` — 12 sub-cases (4 valid hex, 6 invalid, empty, nil)
  - `TestSentinelErrors` — 4 sentinel errors are distinct
  - `TestColorRePattern` — 5 regex boundary checks
- `api/handler/tag.go` — 9 handlers:
  - Tag CRUD: `Create` (201/409), `List` (with domain_count), `Update`, `Delete`
  - Domain tags: `GetDomainTags`, `SetDomainTags`
  - Bulk: `BulkAction` — supports "update" (fields), "add_tags", "remove_tags";
    500-domain cap; validates required fields per action
  - CSV: `Export` — streams `text/csv` with header row; includes per-domain tags
    (semicolon-delimited); supports project_id/tag_id/lifecycle_state filters
- `api/router/router.go` — registered routes:
  - `POST/GET /tags`, `PUT/DELETE /tags/:id`
  - `GET/PUT /domains/:id/tags`
  - `POST /domains/bulk` (static, before /:id)
  - `GET /domains/export` (static, before /:id)
  - `TagHandler` added to `Deps` struct
- `cmd/server/main.go` — wired `TagStore → tag.Service → TagHandler`
- `web/src/types/tag.ts` — `TagResponse`, `CreateTagRequest`, `UpdateTagRequest`,
  `BulkAction` union, `BulkActionRequest`
- `web/src/api/tag.ts` — full API client (CRUD + domain tags + bulk + export URL)
- `web/src/stores/tag.ts` — Pinia store with all actions + local array updates
- `web/src/views/settings/TagList.vue` — list table (NTag colour preview,
  domain_count, create/edit modals, NPopconfirm delete)
- `web/src/views/domains/DomainList.vue` — tag filter dropdown, selection column
  (`type: 'selection'`), bulk action bar (appears when rows selected — tag add
  via dropdown), CSV export button
- `web/src/views/domains/DomainDetail.vue` — multi-select tag editor in sidebar
  (NSelect multiple, updates via `setDomainTags`)
- `web/src/components/AppTable.vue` — added `v-bind="attrs"` pass-through to
  NDataTable for `checked-row-keys` and other attribute forwarding
- `web/src/router/index.ts` — added `/settings/tags` route
- `web/src/views/layouts/MainLayout.vue` — added "標籤管理" sidebar entry
- `go build ./...` passes; `go test ./internal/tag/...` 3 tests pass;
  `npm run build` zero TypeScript errors

---

### PA.7 — Expiry Dashboard + Notifications **(Opus)**

**Owner**: **Opus** — correctness-critical (expiry alerts must not miss or spam)
**Depends on**: PA.3 (domain expiry data), PA.4 (SSL cert expiry data)
**Reads first**: `docs/analysis/UPTIME_KUMA_ANALYSIS.md` §4 "Notification
Architecture", `docs/analysis/NOMULUS_ANALYSIS.md` §8 "Computed expiry_status",
CLAUDE.md Critical Rule #8 (alert dedup)

**Context**: The most valuable automated action in Phase A: detect domains and
certs approaching expiry and alert operators BEFORE they expire. This is the
first real use of the notification system.

**Scope (in)**:

- asynq task: `domain:expiry_check` (runs daily at 07:00 UTC):
  - For each active domain with `expiry_date IS NOT NULL`:
    - Compute `expiry_status` using logic from NOMULUS_ANALYSIS.md §8
    - If status CHANGED from previous value → persist + trigger notification
  - For each `ssl_certificates` with status change → trigger notification
  - Notification thresholds: 90d (info), 30d (warning), 7d (urgent), expired (critical)
  - Batch multiple domains into one notification message (Critical Rule #8)

- `internal/domain/expiry.go`:
  - `ComputeExpiryStatus(expiryDate, graceEndDate, now) string`
  - `CheckAllExpiry(ctx) ([]ExpiryStateChange, error)`
  - `ExpiryStateChange`: `{ DomainID, FQDN, OldStatus, NewStatus, ExpiryDate }`

- Notification integration:
  - Use existing `pkg/notify` (Telegram + Webhook)
  - Message format:
    ```
    ⚠️ Domain Expiry Alert (7 days)
    
    3 domains expiring within 7 days:
    • example.com — expires 2026-04-28 (Namecheap)
    • test.io — expires 2026-04-29 (GoDaddy)
    • foo.cn — expires 2026-04-30 (Namecheap)
    
    1 SSL certificate expiring within 7 days:
    • api.example.com — cert expires 2026-04-27 (Let's Encrypt)
    ```
  - Dedup: same domain + same threshold → notify once per day max

- Dashboard frontend:
  - `web/src/views/dashboard/ExpiryDashboard.vue` (new page, or widget on main dashboard):
    - Expiry bands: expired (red), 7d (orange), 30d (yellow), 90d (blue)
    - Domain count per band
    - Clickable → filtered domain list
    - SSL cert expiry: same bands
    - Calendar view: upcoming expirations on a month calendar
  - Sidebar: "Expiring (N)" badge when N > 0

**Scope (out)**:

- Auto-renewal triggering (we notify, humans renew)
- Multi-channel notification configuration UI (Phase C)
- Complex alert rules (Phase C alert engine)
- Custom notification schedules per domain

**Deliverables**:

- `domain:expiry_check` asynq worker handler
- `ComputeExpiryStatus` logic with unit tests
- Notification dispatch (batch messages, dedup)
- Expiry dashboard page with bands + calendar
- Sidebar badge for expiring count

**Acceptance**:

- Domain with expiry_date = today + 25 days → `expiry_status = "expiring_30d"`
- Domain with expiry_date = today - 2 days → `expiry_status = "expired"`
- Domain with expiry_date = today - 2 days + grace_end_date = today + 28 days
  → `expiry_status = "grace"`
- Worker runs → detects status changes → sends Telegram notification
- Notification batches: 5 domains expiring in 30d → ONE message with all 5
- Same domain, same status → no duplicate notification within 24h
- Dashboard shows correct counts per band
- Calendar shows dots on days with expirations
- `go test -race ./internal/domain/...` passes
- Worker idempotent: running twice in same day → no duplicate notifications

---

### PA.8 — Import Queue

**Owner**: Sonnet
**Depends on**: PA.1 (domain_import_jobs table), PA.2 (registrar_accounts),
PA.3 (domain creation with asset fields)
**Reads first**: `docs/analysis/DOMAINMOD_ANALYSIS.md` §3 "Pattern 3: Import
Queue Pipeline", §8 "Import Queue Design"

**Context**: Enterprises have hundreds/thousands of existing domains. Manual
entry is impractical. This task provides CSV upload and (optionally) registrar
API sync for bulk domain onboarding.

**Scope (in)**:

- `internal/importer/service.go`:
  - `ImportFromCSV(ctx, input) (*DomainImportJob, error)`:
    - Parse CSV (columns: fqdn, expiry_date, auto_renew, registrar_account_id, notes)
    - Validate each row (FQDN format, date format)
    - Dedup against existing domains (skip if FQDN exists)
    - Create `domain_import_jobs` row with status tracking
    - Enqueue asynq task: `domain:import`
  - `GetImportJob(ctx, id) (*DomainImportJob, error)`
  - `ListImportJobs(ctx) ([]DomainImportJob, error)`

- asynq task: `domain:import`:
  - Process import job row by row
  - For each valid, non-duplicate domain: create via domain service
  - Update job: `imported_count`, `skipped_count`, `failed_count`
  - On completion: set status = "completed" + `completed_at`
  - On fatal error: set status = "failed" + `error_details` JSONB

- `api/handler/import.go`:
  - `POST /api/v1/domains/import` — multipart form upload (CSV file) +
    metadata (project_id, default registrar_account_id, default dns_provider_id)
  - `GET /api/v1/domains/import/jobs` — list import jobs (with status)
  - `GET /api/v1/domains/import/jobs/:id` — get job detail (counts + errors)

- CSV format definition:
  ```csv
  fqdn,expiry_date,auto_renew,registrar_account_id,dns_provider_id,tags,notes
  example.com,2027-03-15,true,1,2,"production;core",Main site
  test.io,2026-12-01,false,1,2,"staging",Test domain
  ```

- Frontend:
  - `web/src/views/domains/ImportWizard.vue`:
    - Step 1: Upload CSV file
    - Step 2: Preview parsed data (table showing first 10 rows)
    - Step 3: Set defaults (project, registrar_account, dns_provider)
    - Step 4: Confirm + start import
    - Step 5: Progress (polling job status: imported/skipped/failed counts)
  - Import job history page: list of past imports with status

**Scope (out)**:

- Live registrar API sync (pull domain list from Namecheap API) — future enhancement
- Conflict resolution UI (domain exists in different project)
- CSV template download (future nice-to-have)
- Export + re-import round-trip

**Deliverables**:

- Import service with CSV parsing + validation + dedup
- asynq worker handler for async processing
- API endpoints (upload + job status)
- Frontend import wizard (5-step flow)
- Import job history page

**Acceptance**:

- Upload CSV with 50 domains → import job created, returns job ID
- Job processes async → 47 imported, 3 skipped (already exist)
- `GET /api/v1/domains/import/jobs/:id` shows correct counts
- Invalid FQDN in CSV → recorded in `failed_count` + `error_details`
- Imported domains have correct: project_id, registrar_account_id, tld (auto-extracted), expiry_date
- Frontend wizard shows progress bar during import
- Import with 0 valid rows → job status = "failed"
- `go test ./internal/importer/...` passes
- Large CSV (1000 rows) completes within 60 seconds

---

## Phase A Effort Estimate

| # | Task | Owner | Lo | Hi | Risk | Notes |
|---|---|---|---|---|---|---|
| PA.1 | Schema + Models + Store | **Opus** | 1.5 | 2.5 | 🟡 | Large schema, many tables; careful migration design |
| PA.2 | Registrar + Provider CRUD | Sonnet | 1.5 | 2.5 | 🟢 | Standard CRUD, low risk |
| PA.3 | Domain Asset Extension | Sonnet | 2.0 | 3.5 | 🟡 | Many fields; TLD extraction edge cases; transfer flow |
| PA.4 | SSL Cert Tracking | Sonnet | 1.0 | 2.0 | 🟢 | TLS connect is straightforward |
| PA.5 | Fee Schedule + Cost | Sonnet | 1.0 | 2.0 | 🟢 | Auto-calculation needs careful testing |
| PA.6 | Tags + Bulk Ops | Sonnet | 1.5 | 2.5 | 🟢 | Bulk update needs transaction safety |
| PA.7 | Expiry Dashboard + Alerts | **Opus** | 1.5 | 3.0 | 🟡 | Notification batching + dedup logic |
| PA.8 | Import Queue | Sonnet | 1.5 | 2.5 | 🟡 | CSV parsing edge cases; large file handling |

**Task sum**: Lo = 11.5 days / Hi = 20.5 days

**Integration friction**: +2–3 days (domain service wiring, frontend state management)

| | Work days | Calendar weeks |
|---|---|---|
| **Optimistic** | 13.5 days | ~3 weeks |
| **Mid-range** | 18 days | ~4 weeks |
| **Pessimistic** | 23.5 days | ~5 weeks |

### Recommended work order

```
Week 1:  PA.1 (schema — blocks everything)
Week 2:  PA.2 (registrar/provider CRUD) + PA.3 start (domain extension)
Week 3:  PA.3 finish + PA.4 (SSL) + PA.5 (cost) — parallel
Week 4:  PA.6 (tags/bulk) + PA.7 (expiry dashboard + alerts)
Week 5:  PA.8 (import) + integration testing + polish
```

---

## Scope Creep Warnings

| Temptation | Truth |
|---|---|
| "PA.2 should validate credentials by testing the API connection" | Validation is Phase B work (DNS sync). Store credentials now, validate later. |
| "PA.3 should support multi-level TLD extraction perfectly" | Use a TLD list library (publicsuffix). Don't hand-roll extraction logic. |
| "PA.4 should track the full certificate chain" | Only track the leaf cert. Chain validation is not our job. |
| "PA.5 should support multi-currency conversion" | Store in original currency. Display as-is. Conversion is future. |
| "PA.6 should have tag-based automation rules" | Tags are for filtering/grouping only in Phase A. Automation is Phase C. |
| "PA.7 should support configurable notification schedules" | Fixed thresholds (90/30/7/expired) are fine. Custom schedules are Phase C. |
| "PA.8 should auto-sync from registrar APIs" | CSV import only. API sync is a future enhancement after providers are proven. |

---

## References

- `docs/DOMAIN_ASSET_LAYER_DESIGN.md` — Detailed schema + design decisions
- `docs/ARCHITECTURE_ROADMAP.md` — Phase A in context of full platform
- `docs/analysis/DOMAINMOD_ANALYSIS.md` — Asset data model reference
- `docs/analysis/NOMULUS_ANALYSIS.md` — Lifecycle + expiry + transfer patterns
- `docs/analysis/DNSCONTROL_ANALYSIS.md` — Provider interface (used in PA.2 wiring)
- `docs/FRONTEND_GUIDE.md` — Vue 3 component conventions
- `CLAUDE.md` — Tech stack, coding standards, critical rules
