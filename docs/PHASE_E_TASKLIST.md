# PHASE_E_TASKLIST.md — CDN Account Management & Domain Asset Enrichment (PE.1 ✅ PE.2 ✅)

> **Created 2026-04-26.** This document is the authoritative work order for
> Phase E (CDN Account Management & Domain Asset Enrichment).
>
> **Context**: Analysis of existing domain management system screenshots
> revealed missing CDN account management, origin IP, and domain purpose fields.
> Phase E fills those gaps at the same tier as Registrar and DNS Provider.
>
> **Pre-requisite**: Phase A PA.1–PA.2 complete (domain asset management, tags).
>
> **Audience**: Claude Code sessions. All tasks are standard-complexity CRUD;
> Sonnet is appropriate for all PE tasks.

---

## Phase E — Definition of Scope

Phase E adds **CDN account management** and **domain asset enrichment**:
unified cloud-account management for CDN/acceleration vendors (parallel to
Registrar / DNS Provider), plus extra metadata fields on domains that are
visible in daily operations but absent from the initial schema.

### What "Phase E done" looks like (acceptance demo)

```
1. Operator opens 資產管理 → CDN 供應商管理
2. Sees 8 pre-seeded providers: Cloudflare, 聚合, 網宿, 白山雲, 騰訊雲CDN,
   華為雲CDN, 阿里雲CDN, Fastly  (+ can add custom "其他")
3. Clicks into Cloudflare provider → creates account "主播 Cloudflare"
   with credentials { "api_key": "...", "zone_id": "..." }
4. Opens domain detail → assigns cdn_account_id = that account
5. Domain list now shows CDN column: "Cloudflare / 主播 Cloudflare"
6. Operator can see origin IP and domain purpose in domain detail
```

---

## Task Cards

---

### PE.1 — CDN Provider & Account CRUD ✅ (完成 2026-04-26)

**Goal**: `cdn_providers` + `cdn_accounts` tables with full API + Vue frontend,
parallel to the Registrar/DNS Provider pattern.

**Deliverables**:

| Layer | File | Status |
|-------|------|--------|
| Migration | `migrations/000001_init.up.sql` (cdn_providers, cdn_accounts tables + seed) | ✅ |
| Migration | `migrations/000001_init.down.sql` (DROP cdn_accounts, cdn_providers) | ✅ |
| Store | `store/postgres/cdn.go` (CDNStore + 12 methods + sentinel errors) | ✅ |
| Service | `internal/cdn/service.go` (Service + allowedProviderTypes + validation) | ✅ |
| Tests | `internal/cdn/service_test.go` (14 unit tests, nil-store pattern) | ✅ |
| Handler | `api/handler/cdn.go` (CDNHandler + 11 handlers) | ✅ |
| Router | `api/router/router.go` (/cdn-providers + /cdn-accounts route groups) | ✅ |
| Wire | `cmd/server/main.go` (cdnStore + cdnSvc + cdnHandler wiring) | ✅ |
| API Types | `web/src/api/cdn.ts` (CDN_PROVIDER_TYPES + cdnApi + TS interfaces) | ✅ |
| Store | `web/src/stores/cdn.ts` (useCDNStore Pinia store) | ✅ |
| View | `web/src/views/cdn-providers/CDNProviderList.vue` | ✅ |
| View | `web/src/views/cdn-providers/CDNProviderDetail.vue` | ✅ |
| Router | `web/src/router/index.ts` (/cdn-providers, /cdn-providers/:id routes) | ✅ |
| Nav | `web/src/views/layouts/MainLayout.vue` (CDN 供應商管理 nav item) | ✅ |

**Key design decisions**:
- `allowedProviderTypes` map in service layer (not DB enum) for easy extension
- `provider_type` + `name` unique together, not globally unique — allows "Cloudflare Prod" and "Cloudflare Dev"
- `credentials` stored as JSONB `'{}'::jsonb` default; service defaults nil → `{}`
- `parseParamID(c, param)` used (not `parseID`) to avoid collision with notification.go's `parseID(c)`
- Unit tests use nil-store pattern: only validation-error paths are tested; store calls would panic (not reached)

**Known gaps / deferred**:
- `credentials` stored as JSONB plaintext. PE.1 defers encryption; see MASTER_ROADMAP §15.2 for AES-256-GCM recommendation
- No role-based write protection on credentials (PE.2 or security pass)

---

### PE.2 — Domain Asset Field Enrichment ✅ (完成 2026-04-26)

**Goal**: Add `cdn_account_id`, `origin_ips`, and `domain_purpose` to the
`domains` table. Wire up in domain detail UI.

**Scope**:
- New migration: `ALTER TABLE domains ADD COLUMN cdn_account_id BIGINT REFERENCES cdn_accounts(id)`
- Add `origin_ips TEXT[]` and `domain_purpose VARCHAR(32)` columns
- Update `DomainDetail.vue` — show CDN account (read) + allow assignment
- Update `DomainList.vue` — add CDN column (vendor name + account name)
- Add `cdnApi.listAllAccounts()` to domain store for account picker

**Acceptance criteria**:
- Domain detail shows assigned CDN account (name + provider type)
- Domain detail allows changing CDN account from dropdown
- Domain list shows CDN vendor name in new column (empty = `-`)
- origin_ips stored as array, displayed as comma-separated in UI

**Estimated steps**: 5
1. Migration: add columns to domains
2. Store: update GetDomainByID, ListDomains queries
3. Service: update UpdateDomain input to include cdn_account_id + origin_ips + purpose
4. Handler: update DomainResponse DTO + update endpoint
5. Frontend: DomainDetail CDN assignment + DomainList CDN column

---

### PE.3 — Domain List UI Strengthening ✅ (完成 2026-04-26)

**Goal**: Richer domain list — CDN column, Registrar column, purpose badge, origin IP tooltip.

**Deliverables**:

| Layer | File | Status |
|-------|------|--------|
| Store | `store/postgres/domain.go` — `DomainListRow` struct + `ListEnriched` + `CDNProviderID`/`Purpose` filters | ✅ |
| Service | `internal/lifecycle/service.go` — `toListFilter()` helper + `ListEnriched` + `ListEnrichedResult` | ✅ |
| Service | `internal/tag/service.go` — `ExportDomainsEnriched` | ✅ |
| Handler | `api/handler/domain.go` — `List` switched to `ListEnriched`; `domainListItemResponse`; new query params | ✅ |
| Handler | `api/handler/tag.go` — `Export` rewritten with new columns + `ExportDomainsEnriched` | ✅ |
| API Types | `web/src/api/domain.ts` — `cdn_provider_id` + `purpose` added to `DomainListParams` | ✅ |
| TS Types | `web/src/types/domain.ts` — `registrar_name`, `cdn_account_name`, `cdn_provider_type` added | ✅ |
| View | `web/src/views/domains/DomainList.vue` — Registrar/CDN/Purpose columns + CDN+purpose filters | ✅ |
| Tests | `internal/lifecycle/list_enriched_test.go` — 18 unit tests | ✅ |

**Key design decisions**:
- `DomainListRow` embeds `Domain` anonymously — sqlx scans embedded struct fields directly, no custom mapping needed
- `ListEnriched` uses `enrichedDomainSelect` + `enrichedDomainJoins` constants to avoid SQL ambiguity; all domain columns prefixed with `d.`
- `CDNProviderID` filter uses a subquery (`cdn_account_id IN (SELECT ...)`) so it reuses the existing FK without schema changes
- `toListFilter()` helper extracted to avoid duplication between `List` and `ListEnriched`
- CSV export calls `ExportDomainsEnriched` (10k limit) and adds: purpose, registrar_name, cdn_provider_type, cdn_account_name, origin_ips
- Frontend: `cdnStore.fetchList()` fetches providers for the filter dropdown; `CDN_PROVIDER_TYPES` used for human-readable type labels

---

## Phase E Progress

| Task | Description | Status | Date |
|------|-------------|--------|------|
| PE.1 | CDN 帳號管理 | ✅ 完成 | 2026-04-26 |
| PE.2 | 域名資產欄位補全 | ✅ 完成 | 2026-04-26 |
| PE.3 | 域名列表 UI 強化 | ✅ 完成 | 2026-04-26 |

**Phase E 整體進度**: 3 / 3 完成（100%）
