# PHASE_F_TASKLIST.md — Registrar API Sync (PF.1 ✅)

> **Created 2026-04-26.** This document is the authoritative work order for
> Phase F (Registrar API Sync & Credential Management).
>
> **Context**: Domains table already has `registration_date` and `expiry_date`
> columns but no mechanism to populate them automatically. Registrar accounts
> had a credentials JSONB column but no UI to set it. Phase F fills both gaps.
>
> **Pre-requisite**: Phase A (domain management), PE.1 (CDN pattern reference).
>
> **Audience**: Claude Code sessions. All tasks are standard-complexity;
> Sonnet is appropriate for all PF tasks.

---

## Phase F — Definition of Scope

Phase F adds **Registrar API sync** — the ability to pull domain
registration/expiry dates directly from the registrar's API (starting with
GoDaddy) and write them into the `domains` table. It also adds the missing
**credential management UI** for registrar accounts.

### What "Phase F done" looks like (acceptance demo)

```
1. Operator opens 資產管理 → 供應商管理 → GoDaddy
2. Clicks into GoDaddy account → clicks "設定憑證"
3. Enters api_key + api_secret → saves
4. Clicks "同步域名" on the account
5. Modal shows: total=50, updated=45, not_found=5 (domains in GoDaddy
   but not yet registered in our system)
6. Opens Domain detail → registration_date and expiry_date are now populated
```

---

## Task Cards

---

### PF.1 — Registrar API Sync ✅ (完成 2026-04-26)

**Goal**: GoDaddy registrar provider + credential management UI + sync endpoint.

**Deliverables**:

| Layer | File | Status |
|-------|------|--------|
| Provider interface | `pkg/provider/registrar/provider.go` | ✅ |
| GoDaddy provider | `pkg/provider/registrar/godaddy.go` | ✅ |
| GoDaddy tests | `pkg/provider/registrar/godaddy_test.go` (24 tests) | ✅ |
| Store | `store/postgres/domain.go` — `UpdateDomainDates()` method | ✅ |
| Service | `internal/registrar/service.go` — `SyncAccount()` + `SyncResult` + `domainDateUpdater` interface | ✅ |
| Service tests | `internal/registrar/sync_test.go` (10 tests, closure-based mocks) | ✅ |
| Handler | `api/handler/registrar.go` — `SyncAccount` handler | ✅ |
| Router | `api/router/router.go` — `POST /registrar-accounts/:id/sync` | ✅ |
| Wire | `cmd/server/main.go` — `domainStore` passed to `registrar.NewService`; side-effect imports | ✅ |
| API types | `web/src/api/registrar.ts` — `syncAccount()` | ✅ |
| TS types | `web/src/types/registrar.ts` — `SyncResult`, `SyncItemError`, `GoDaddyCredentials` | ✅ |
| Store | `web/src/stores/registrar.ts` — `syncAccount()` action | ✅ |
| View | `web/src/views/registrars/RegistrarDetail.vue` — credentials modal + sync button + result modal | ✅ |

**Key design decisions**:

- `pkg/provider/registrar` mirrors the existing `pkg/provider/dns` pattern:
  `Provider` interface + `Factory` function + global registry + `init()` registration
- `domainDateUpdater` interface (single method) in `internal/registrar` decouples
  the service from `*postgres.DomainStore` — makes sync tests fast (no DB needed)
- `UpdateDomainDates` matches by `fqdn + registrar_account_id` — a domain in GoDaddy
  that is NOT bound to this account in our DB returns `updated=false` and goes to
  `SyncResult.NotFound`, not an error
- `SyncAccount` is **non-fatal per domain** — DB errors on individual domains are
  accumulated in `SyncResult.Errors` and do not abort the full sync
- `SyncResult.NotFound` is intentional — these are domains the operator has in their
  registrar but hasn't registered in our platform yet; sync reports them, does NOT
  auto-create
- GoDaddy uses `sso-key KEY:SECRET` header (not Bearer token); auth errors map to
  `ErrUnauthorized` sentinel
- GoDaddy list pagination uses `marker` (last FQDN of previous page), page size 500
- Credentials UI: GoDaddy shows typed form (api_key, api_secret, environment);
  other/unknown api_types fall back to raw JSON textarea
- Sync result modal shows 3 statistics + scrollable not_found + errors list

**Known gaps / deferred**:

- `credentials` stored as JSONB plaintext — encryption (AES-256-GCM) deferred to
  security pass (see MASTER_ROADMAP §15.2 which also applies to CDN credentials)
- Only GoDaddy is implemented. Other registrars (Namecheap, 阿里雲, 騰訊雲) follow
  the same `pkg/provider/registrar` pattern; add `Register("namecheap", ...)` in
  their own file with `init()`.
- No scheduled/automatic sync (cron job) yet — operator triggers sync manually.
  A future `TypeRegistrarSync = "registrar:sync"` asynq task can automate this.

---

## Phase F Progress

| Task | Description | Status | Date |
|------|-------------|--------|------|
| PF.1 | Registrar API 同步 + 憑證管理 UI | ✅ 完成 | 2026-04-26 |

**Phase F 整體進度**: 1 / 1 完成（100%）
