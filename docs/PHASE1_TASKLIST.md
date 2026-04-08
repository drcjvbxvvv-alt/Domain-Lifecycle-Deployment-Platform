# PHASE1_TASKLIST.md — Phase 1 Work Order

> **Audience**: Claude Code sessions implementing Phase 1 (both Sonnet and Opus).
> **Status**: Pre-implementation alignment complete (ADR-0001, ADR-0002 merged).
> Phase 1 may begin. This document is the single entry point — read it before
> starting any task, and follow the dependency graph strictly.

---

## Phase 1 — Definition of Scope

Phase 1 ships the **control plane skeleton** only. At the end of Phase 1 an
operator can:

1. Log in to the management console (username/password, not email).
2. Create / edit / list **projects** and their **prefix rules**.
3. Create / edit / list **main domains** and auto-generated **subdomains**.
4. Observe domain **status transitions** through the state machine and its
   audit history.
5. See all data in the Vue 3 frontend (list views for projects and domains).

Phase 1 does **NOT** touch any real infrastructure. No DNS records are
actually created, no CDN is configured, no nginx conf is rendered, no SVN
commit happens, no probe fires, no Telegram alert is sent. The system is
wired end-to-end at the **API + DB + state-machine level** so that Phase 2
can plug in executors without re-designing anything.

### What is explicitly OUT of Phase 1

Do **not** implement any of the following as part of Phase 1. They belong to
Phase 2 or later. If a P1 task tempts you to start on one, stop and re-read
this section.

| Subsystem | Path | Phase |
|---|---|---|
| Release executor (canary, shard, rollback) | `internal/release/` (executor logic) | Phase 2 |
| Auto-switcher (Redis + Postgres double lock, ADR-0002 D1) | `internal/switcher/` | Phase 2 |
| Probe monitoring + scanner binary | `internal/probe/`, `cmd/scanner/` | Phase 2 |
| Standby pool warmup / promote / block | `internal/pool/` | Phase 2 |
| Alert deduplication + batch notify | `internal/alert/` | Phase 2 |
| nginx conf template rendering | `pkg/template/` | Phase 2 |
| SVN Agent client + deploy | `pkg/svnagent/` | Phase 2 |
| Telegram / webhook notify | `pkg/notify/` | Phase 2 |
| **Concrete** DNS/CDN vendor clients (Cloudflare, Aliyun, Tencent, …) | `pkg/provider/dns/*.go`, `pkg/provider/cdn/*.go` | Phase 2 |
| TimescaleDB `probe_results` write path | `store/timescale/` | Phase 2 |

**Note on `releases` table in Phase 1**: the table exists in the initial
migration (per `DATABASE_SCHEMA.md`), and P1.4 inserts rows into it for
rebuild requests, but **no worker consumes them**. That is intentional — the
release executor is Phase 2 work, and the Phase 1 stub exists so the
soft-freeze contract in ADR-0002 D3 is enforceable from day one.

---

## Dependency Graph

```
                         P1.1 (scaffold)
                              │
                              ▼
                         P1.2 (migrations)
                              │
          ┌───────┬───────────┼───────────┬───────┐
          ▼       ▼           ▼           ▼       ▼
        P1.3    P1.7        P1.8        P1.5    P1.4
        Auth   asynq      Providers  State M.   Project
               worker    (interface) (Opus)     CRUD +
                                                soft-freeze
          │       │           │           │       │
          └───────┴───────────┴───────────┴───────┘
                              │
                              ▼
                         P1.6 (Main Domain CRUD + Subdomain)
                          [requires P1.5 Transition()]
                              │
                              ▼
                         P1.9 (Frontend list views)
                          [requires P1.3 auth + P1.6 API]
```

### Critical path

`P1.1 → P1.2 → P1.5 → P1.6 → P1.9`

**P1.5 is the bottleneck.** It is the single task assigned to Opus in this
phase. Start P1.5 **as early as P1.2 finishes**, in parallel with
P1.3/P1.4/P1.7/P1.8 — do not wait for the Sonnet tasks to complete. The
earlier Opus lands `Transition()` + the race test + the `make
check-status-writes` CI gate, the earlier P1.6 can unblock.

### Parallelization rules

- After P1.2 merges, **five tasks may run in parallel**: P1.3, P1.4, P1.5,
  P1.7, P1.8. They touch disjoint packages and do not share write paths.
- P1.6 must wait for **both P1.5 and P1.4** (it uses `Transition()` and it
  reads `prefix_rules` for subdomain auto-generation).
- P1.9 must wait for **P1.3 and P1.6** (login API + domain list API).

---

## Task Cards

Each card specifies: owner model, scope (in), scope (out), dependencies,
deliverable artifacts, acceptance criteria, and the docs to read first.
**Do not deviate from "scope (out)" — those items belong to Phase 2.**

---

### P1.1 — Scaffold Go repo + tooling

**Owner model**: Sonnet
**Depends on**: (none — first task)
**Reads first**: `CLAUDE.md` §"Project Structure" and §"Go Coding Standards"; `README.md`

**Scope (in)**:
- Verify the existing directory skeleton is complete (`cmd/`, `internal/`,
  `pkg/`, `store/`, `api/`, `migrations/`, `templates/`, `web/`, `configs/`,
  `deploy/`). Create any missing subdirs referenced by `CLAUDE.md`.
- `cmd/server/main.go`: Gin server boot, graceful shutdown, signal handling
  (`DEVELOPMENT_PLAYBOOK.md` §8 "Graceful Shutdown").
- `cmd/worker/main.go`: asynq server boot (queue list will be filled in by
  P1.7 — leave a clearly marked `// TODO(P1.7)` stub if needed).
- `cmd/migrate/main.go`: thin wrapper around
  `github.com/golang-migrate/migrate/v4` for `up`, `down`, `version`.
- `cmd/scanner/main.go`: empty `func main() { log.Println("scanner: not implemented in Phase 1") }`. Do NOT implement probe logic.
- Internal shared packages:
  - `internal/bootstrap/` (or `internal/app/`): Viper config loader,
    Zap logger factory, sqlx PostgreSQL connection pool, go-redis client,
    asynq client factory. **One** place that constructs these singletons.
  - `configs/config.example.yaml`: full config reference file with every
    key documented (copy from CLAUDE.md env vars section).
- `Makefile`: verify every target in CLAUDE.md §"Makefile Commands" works.
  Update `make test` to run `go test ./...`. Add `make lint` wired to
  `golangci-lint`.
- `go.mod` / `go.sum`: ensure all dependencies referenced by CLAUDE.md are
  present (gin, sqlx, asynq, zap, viper, jwt/v5, uuid). Run `go mod tidy`.
- `.golangci.yml`: minimal config (govet, staticcheck, errcheck, revive).

**Scope (out)**:
- No business logic. No handlers. No store methods. No migrations.
- No frontend changes.
- Do not touch existing web/ code.

**Deliverables**:
- `cmd/server/main.go`, `cmd/worker/main.go`, `cmd/migrate/main.go`,
  `cmd/scanner/main.go` all compile and run.
- `internal/bootstrap/config.go`, `internal/bootstrap/logger.go`,
  `internal/bootstrap/db.go`, `internal/bootstrap/redis.go`,
  `internal/bootstrap/asynq.go`.
- `configs/config.example.yaml`.
- `.golangci.yml`.
- Updated `Makefile`.

**Acceptance**:
- `make build` succeeds, producing 4 binaries.
- `make lint` runs clean.
- `./bin/server` starts, binds to `:8080`, logs a structured "server started"
  line, and exits cleanly on `SIGTERM`.
- `./bin/worker` starts, connects to Redis, logs "worker started" (queue
  list is allowed to be empty at this point).
- `./bin/migrate version` runs (even if it reports "no migrations").

---

### P1.2 — Phase 1 DB migrations

**Owner model**: Sonnet
**Depends on**: P1.1
**Reads first**: `DATABASE_SCHEMA.md` (entire file), ADR-0002 D3 (`releases.kind`), ADR-0001 D4 (pool state rename), CLAUDE.md §"Database Migrations"

**Scope (in)**:
- Write `migrations/000001_init.up.sql` and `migrations/000001_init.down.sql`
  containing every table defined in `DATABASE_SCHEMA.md`:
  - `projects`, `prefix_rules`, `main_domains`, `subdomains`,
    `main_domain_pool` (with all 6 pool states: `pending`, `warming`, `ready`,
    `promoted`, `blocked`, `retired`), `domain_state_history`, `switch_history`,
    `servers`, `releases` (with `kind VARCHAR(20) NOT NULL DEFAULT 'deploy'`,
    per ADR-0002 D3), `release_shards`, `domain_tasks` (with `chk_task_type`
    CHECK constraint), `conf_snapshots`, `alert_events`, `users`, `audit_logs`.
  - Include all indexes specified in `DATABASE_SCHEMA.md`.
- Write `migrations/000002_timescale.up.sql` / `.down.sql` creating the
  `probe_results` hypertable (TimescaleDB extension + `create_hypertable`).
  The table exists but will not be written to in Phase 1.
- All tables follow the conventions in `DATABASE_SCHEMA.md` lines 15-21
  (`id BIGSERIAL PRIMARY KEY`, `uuid`, timestamps, `deleted_at`, etc.).

**Pre-launch exception rule**: `000001_init.up.sql` may still be edited in
place during Phase 1 per ADR-0001 + ADR-0002. **After Phase 1 cutover this
window closes** — do not assume you can keep doing this.

**Scope (out)**:
- No seed data (users are seeded by P1.3).
- No stored procedures, no triggers.
- Do not add tables not in `DATABASE_SCHEMA.md`.

**Deliverables**:
- `migrations/000001_init.up.sql`
- `migrations/000001_init.down.sql`
- `migrations/000002_timescale.up.sql`
- `migrations/000002_timescale.down.sql`

**Acceptance**:
- `make migrate` applies cleanly against an empty PostgreSQL 16 + TimescaleDB
  database.
- `make migrate-down` rolls back cleanly.
- `\d releases` in psql shows the `kind` column with default `'deploy'`.
- `\d main_domain_pool` in psql shows the `chk_pool_status` CHECK with all
  6 states.
- `\d domain_tasks` shows the `chk_task_type` CHECK constraint.
- `\dt` lists every table enumerated above.

---

### P1.3 — Auth: login, JWT, RBAC

**Owner model**: Sonnet
**Depends on**: P1.2 (needs `users` table)
**Reads first**: CLAUDE.md §"API Conventions", `DEVELOPMENT_PLAYBOOK.md` §7 "Login identifier", recent commit `af8edbc` ("replace email field with username")

**Scope (in)**:
- `store/postgres/user.go`: `GetByUsername`, `GetByID`, `Create`, `UpdatePassword`.
  **Note**: the login identifier is `username`, NOT `email` — this was
  deliberately changed (see `feat(login): replace email field with username
  for internal system`). Do not reintroduce `GetByEmail`.
- `internal/auth/`:
  - `service.go`: `Login(ctx, username, password) (tokenPair, error)`,
    `Refresh`, password hashing with `bcrypt`.
  - `jwt.go`: sign / verify using `golang-jwt/jwt/v5`, claims include
    `user_id`, `username`, `role`, `exp`.
- `api/middleware/auth.go`: Bearer token extraction, JWT verification,
  attaches `user_id` / `role` to `gin.Context`.
- `api/middleware/rbac.go`: role check middleware (`RequireRole("admin")`,
  `RequireRole("operator")`). Phase 1 supports two roles: `admin`, `operator`.
- `api/handler/auth.go`: `POST /api/v1/auth/login`, `POST /api/v1/auth/refresh`,
  `GET /api/v1/auth/me`.
- A minimal seed user created via `cmd/migrate` flag `-seed-admin` OR a
  one-shot SQL script in `deploy/seed/admin.sql`. Document which you chose
  in the handler file header.

**Scope (out)**:
- No OAuth, no SSO, no email verification, no password reset flow.
- No audit logging beyond what `audit_logs` needs (the audit write itself
  is fine, but do not build an audit query API in P1.3).
- No rate limiting (that's a separate middleware, defer to Phase 2 unless
  trivial).

**Deliverables**:
- `store/postgres/user.go`
- `internal/auth/service.go`, `internal/auth/jwt.go`
- `api/middleware/auth.go`, `api/middleware/rbac.go`
- `api/handler/auth.go`
- Seed mechanism for the first admin user
- Unit tests for JWT sign/verify and password hashing

**Acceptance**:
- `POST /api/v1/auth/login` with `{username, password}` returns a JWT.
- `GET /api/v1/auth/me` with `Authorization: Bearer <token>` returns user
  info; without a token returns 401.
- A handler protected by `RequireRole("admin")` returns 403 for an operator
  token.
- `grep -r 'GetByEmail' internal/ store/ api/` returns **zero** results.
- `go test ./internal/auth/...` passes.

---

### P1.4 — Project CRUD + Prefix Rules soft-freeze

**Owner model**: Sonnet
**Depends on**: P1.2, P1.7 (needs asynq client to enqueue rebuild release)
**Reads first**: ADR-0002 D3 (soft-freeze), `DEVELOPMENT_PLAYBOOK.md` §5.5 and §7 (rebuild flow), CLAUDE.md Critical Rule #9

**Scope (in)**:
- `store/postgres/project.go`: CRUD for `projects` table.
- `store/postgres/prefix_rule.go`: CRUD for `prefix_rules`, plus
  `IsInUse(ctx, projectID, prefix) (bool, error)` — returns true if any row
  in `subdomains` references this `(project_id, prefix)` pair.
- `internal/project/service.go`:
  - `Create`, `Update` (metadata-only fields safe to edit anytime),
    `List`, `Get`, `Delete` (soft delete).
  - `UpdatePrefixRule(ctx, projectID, prefix, req)`:
    - Load current rule.
    - Compare runtime fields (`dns_provider`, `cdn_provider`, `nginx_template`,
      `html_template`). If none changed → apply the update in a normal
      transaction and return.
    - If any runtime field changed AND `IsInUse` returns true AND
      `req.Rebuild != true` → return `ErrPrefixRuleDriftRequiresRebuild`.
    - If any runtime field changed AND `req.Rebuild == true` →
      `UpdateWithRebuild()`.
  - `UpdateWithRebuild(ctx, projectID, prefix, req) (*Release, error)`:
    - In **one transaction**:
      1. Update the `prefix_rules` row.
      2. `INSERT INTO releases (kind, project_id, prefix, reason, created_by, ...) VALUES ('rebuild', ...)` — capture the affected subdomain IDs.
      3. Commit.
    - **After** commit: `asynq.Enqueue(TypeReleaseRebuildPrefix, payload)` where
      the payload carries the release ID. Register the task type in
      `internal/tasks/types.go` but do **not** implement its handler — leave
      a `// TODO(Phase2): release executor` stub in the worker that logs
      and acks.
- `api/handler/project.go`: handlers for all the above.
- `api/handler/prefix_rule.go`: `GET /api/v1/projects/:id/prefix-rules`,
  `POST` (create), `PATCH /api/v1/projects/:id/prefix-rules/:prefix` (the
  soft-freeze aware path from §7). On `ErrPrefixRuleDriftRequiresRebuild`
  return HTTP 409 with the error code `40902` and the exact message from
  playbook §7.

**Scope (out)** — this is the most important scope boundary in Phase 1:
- **Do NOT implement the release executor.** Phase 1 only writes the
  `releases` row and enqueues an asynq task. The handler for that task is
  a Phase 2 deliverable.
- **Do NOT render nginx conf, call any provider, call SVN, run any probe,
  or mutate `main_domains.status`**. The rebuild "happens" only as a row
  in the `releases` table plus a queued asynq task.
- **Do NOT implement canary, shard sizing, or rollback** (DEVELOPMENT_PLAYBOOK
  §5.5 describes these, but the execution side is Phase 2).
- `UpdateWithRebuild` **must not call** `internal/domain.Service.Transition()`
  — no status changes happen as part of P1.4. All it does is: write
  prefix_rule + write releases row + enqueue task.

**Deliverables**:
- `store/postgres/project.go`, `store/postgres/prefix_rule.go`
- `internal/project/service.go`, `internal/project/prefix_service.go`,
  `internal/project/errors.go` (with `ErrPrefixRuleDriftRequiresRebuild`)
- `api/handler/project.go`, `api/handler/prefix_rule.go`
- `internal/tasks/types.go` containing `TypeReleaseRebuildPrefix` constant
- Worker stub in `cmd/worker/main.go` that registers the task handler and
  just logs + acks
- Unit tests covering: runtime-field drift detection, `IsInUse` behavior,
  409 path, rebuild happy path ending at "release row inserted + task
  enqueued".

**Acceptance**:
- Creating / editing a project through the API works end-to-end.
- Editing a prefix rule's `description` field (metadata) succeeds with 200.
- Editing a prefix rule's `dns_provider` when subdomains exist **without**
  `rebuild: true` returns 409 with code `40902`.
- Editing the same with `rebuild: true` returns 200, inserts a `releases`
  row with `kind='rebuild'`, and enqueues an asynq task.
- `grep -r 'Transition' internal/project/` returns **zero** results.
- Unit tests pass.

---

### P1.5 — Domain state machine + `Transition()` (Opus bottleneck task)

**Owner model**: **Opus** (do not hand this to Sonnet)
**Depends on**: P1.2 (needs `main_domains` and `domain_state_history` tables)
**Runs in parallel with**: P1.3, P1.4, P1.7, P1.8 — start as soon as P1.2 lands.
**Reads first**: CLAUDE.md §"Domain State Machine" and Critical Rule #8,
ADR-0002 D2, ADR-0002 E2 (CI grep gate)

**Scope (in)**:
- `internal/domain/statemachine.go`:
  - `var validTransitions = map[string][]string{...}` — exactly the map in
    CLAUDE.md §"Domain State Machine". Do not add or remove edges.
  - `func CanTransition(from, to string) bool`
  - Sentinel errors: `ErrInvalidTransition`, `ErrStatusRaceCondition`,
    `ErrDomainNotFound`.
- `store/postgres/domain.go`:
  - `func updateStatusTx(ctx, tx, id, expectedFrom, to string) error` — the
    **only** function in the entire codebase that issues
    `UPDATE main_domains SET status`. Unexported. Called only by
    `Transition()`.
  - `func insertStateHistoryTx(ctx, tx, entry DomainStateHistoryEntry) error`.
- `internal/domain/service.go`:
  - `func (s *Service) Transition(ctx, id int64, from, to, reason, triggeredBy string) error`:
    1. `BeginTxx`
    2. `SELECT status FROM main_domains WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`
    3. If not found → `ErrDomainNotFound`
    4. If current != `from` → `ErrStatusRaceCondition`
    5. If `!CanTransition(current, to)` → `ErrInvalidTransition`
    6. `updateStatusTx(..., from, to)`
    7. `insertStateHistoryTx(..., {from, to, reason, triggeredBy, at: NOW()})`
    8. `Commit`
  - Conventions for `triggeredBy`: `user:{uuid}`, `system`, `probe:{node}`,
    `switcher`, `release:{uuid}`. Document these at the top of `service.go`.
- `internal/domain/service_test.go`:
  - Table-driven tests for every valid edge in `validTransitions`.
  - Table-driven tests for a sample of invalid edges (must return
    `ErrInvalidTransition`).
  - **Race test**: two goroutines both call `Transition(id, "active",
    "degraded", ...)` concurrently. Exactly one must succeed; the other
    must return `ErrStatusRaceCondition`. Run this test with `go test -race`
    and `-count=50` to shake out flakes.
  - Test that `domain_state_history` receives exactly one row per successful
    transition.

**CI gate** (ADR-0002 E2):
- Add a `Makefile` target `check-status-writes`:
  ```
  check-status-writes:
  	@hits=$$(grep -rn 'UPDATE main_domains SET status' --include='*.go' . | \
  		grep -v 'store/postgres/domain.go' || true); \
  	if [ -n "$$hits" ]; then \
  		echo "ERROR: direct status writes found outside store/postgres/domain.go:"; \
  		echo "$$hits"; exit 1; \
  	fi
  ```
- Add a GitHub Actions (or whatever CI the repo uses) step that runs
  `make check-status-writes`. If CI is not yet configured, document the
  command in `README.md` under a "Pre-merge checks" heading and open a
  TODO for the CI wiring.
- Also verify: `grep -rn 'UPDATE main_domains SET status' --include='*.go' .`
  returns **exactly one** hit, in `store/postgres/domain.go::updateStatusTx`.

**Scope (out)**:
- No caller integration yet (P1.6 wires callers). Do not modify
  `internal/release`, `internal/switcher`, `internal/pool` in this task.
- No business logic beyond the state machine itself.

**Deliverables**:
- `internal/domain/statemachine.go`
- `internal/domain/service.go` (containing `Transition()`)
- `internal/domain/errors.go`
- `store/postgres/domain.go` (with `updateStatusTx` + `insertStateHistoryTx`)
- `internal/domain/service_test.go`
- `Makefile` updated with `check-status-writes` target
- `README.md` updated with pre-merge check instructions (or CI config)

**Acceptance**:
- `go test -race -count=50 ./internal/domain/...` passes green.
- `make check-status-writes` passes.
- `grep -rn 'UPDATE main_domains SET status' --include='*.go' .` shows
  exactly one hit.
- Test coverage on `internal/domain/statemachine.go` and the `Transition()`
  function is ≥ 90%.

**Why Opus**: this is the single write path for the most safety-critical
state in the system. A bug here lets concurrent callers corrupt
`main_domains.status`, bypass the state machine, or silently drop history.
The rest of Phase 1 assumes this works correctly.

---

### P1.6 — Main Domain CRUD + Subdomain

**Owner model**: Sonnet
**Depends on**: P1.5 (requires `Transition()`), P1.4 (reads `prefix_rules`)
**Reads first**: `DEVELOPMENT_PLAYBOOK.md` §1 (API endpoint pattern), CLAUDE.md Critical Rule #1 ("prefix determines everything")

**Scope (in)**:
- `store/postgres/domain.go` (extend P1.5's file): `CreateTx`, `GetByID`,
  `GetByDomain`, `ListByProject` (cursor pagination per playbook §8
  "Pagination"), `SoftDelete`. Do **not** add any new status-writing
  method — status always goes through `Transition()`.
- `store/postgres/subdomain.go`: `CreateTx`, `ListByMainDomain`, `GetByFQDN`,
  `SoftDelete`.
- `internal/domain/service.go` (extend P1.5's file):
  - `Create(ctx, req *CreateDomainRequest) (*MainDomain, error)` — follows
    `DEVELOPMENT_PLAYBOOK.md` §1 Step 2 exactly: validate, check duplicates,
    begin tx, insert main_domain with status `inactive`, look up prefix
    rules, insert subdomains for each requested prefix, commit, audit log.
  - `Get`, `List`, `Delete`.
  - **Every caller that changes status** inside this service MUST call
    `s.Transition(...)`. In P1.6 the only such caller is `Delete` (which
    transitions to... actually Phase 1 `Delete` is a soft-delete of the
    row without a status change — confirm this with playbook §6 and match).
- `api/handler/domain.go`: `POST /api/v1/domains`, `GET /api/v1/domains/:id`,
  `GET /api/v1/domains` (list, filter by project_id), `DELETE /api/v1/domains/:id`.
- `api/router/router.go`: wire all routes. Ensure JWT middleware is applied.

**Scope (out)**:
- No `/deploy` endpoint. Deploy is Phase 2 (needs release executor).
- No status-mutating endpoints (`/switch`, `/suspend`, `/resume`). Status
  transitions happen only through the state machine's internal callers in
  Phase 2. In Phase 1 the only status a domain can reach is `inactive`
  (newly created).
- Do not touch DNS or CDN providers — even though P1.8 defines the
  interfaces, P1.6 does not call them.
- No nginx conf rendering, no SVN, no probe.

**Deliverables**:
- `store/postgres/domain.go` (extended), `store/postgres/subdomain.go`
- `internal/domain/service.go` (extended), `internal/domain/dto.go`
- `api/handler/domain.go`
- `api/router/router.go` with all P1 routes wired
- Unit tests for `Create` (happy path + duplicate + unknown prefix) and
  the store methods

**Acceptance**:
- `POST /api/v1/domains` with `{domain, project_id, prefixes}` creates the
  main domain and auto-generates subdomains based on the project's prefix
  rules.
- Newly created domains have status `inactive` and a corresponding row in
  `domain_state_history` is NOT written (initial inserts don't go through
  `Transition` — they set status directly at insert time, which is the
  sole documented exception. Add a comment in `CreateTx` explaining why).

  > Actually — re-verify this decision during P1.6. If CLAUDE.md Critical
  > Rule #8 requires even initial inserts to go through `Transition`, then
  > the insert must use a nil→inactive transition. The current state
  > machine map does not include a `nil`/`""` source state. Whichever
  > approach you pick, document it in a comment at the top of `CreateTx`
  > and in `internal/domain/statemachine.go`, and make sure the
  > `check-status-writes` gate still passes.

- `GET /api/v1/domains?project_id=X` returns a cursor-paginated list.
- `grep -rn 'UPDATE main_domains SET status' --include='*.go' .` still
  returns exactly one hit (P1.5's `updateStatusTx`).
- `make check-status-writes` passes.

---

### P1.7 — asynq worker + queue config

**Owner model**: Sonnet
**Depends on**: P1.1 (worker skeleton)
**Runs in parallel with**: P1.3, P1.4, P1.5, P1.8 (after P1.2)
**Reads first**: `ARCHITECTURE.md` §2.2 (canonical queue layout, ADR-0002 D5), CLAUDE.md §"Task Queue Patterns"

**Scope (in)**:
- `internal/tasks/types.go`: task type constants. Include every `TypeXxx`
  listed in CLAUDE.md §"Task Queue Patterns" even though most handlers are
  Phase 2.
- `cmd/worker/main.go`: finalize the worker boot with the canonical queue
  layout from `ARCHITECTURE.md` §2.2. The queue layout is authoritative —
  this is the **only** place `asynq.Config.Queues` may be defined, per
  ADR-0002 D5.
- Register a `mux.HandleFunc` for every task type in `internal/tasks/types.go`.
  For Phase 2 task types, the handler is a `// TODO(Phase2)` stub that logs
  the task payload at `Info` and returns `nil` (ack). This keeps the worker
  queue drainable in dev without requiring Phase 2 code to exist.
- `internal/bootstrap/asynq.go` (from P1.1): confirm the asynq client is
  exposed and used by handlers that need to enqueue.

**Scope (out)**:
- No real task handlers beyond the P1.4 rebuild stub.
- No scheduler (`asynq.Scheduler` / periodic tasks) — Phase 2.
- No dashboard wiring.

**Deliverables**:
- `internal/tasks/types.go`
- `cmd/worker/main.go` (finalized)
- Worker starts cleanly and processes the P1.4 rebuild task by logging it.

**Acceptance**:
- `./bin/worker` starts, connects to Redis, prints the queue config at boot.
- Enqueuing a `TypeReleaseRebuildPrefix` task via the P1.4 handler results
  in a log line from the worker stub.
- `ARCHITECTURE.md` §2.2 and `cmd/worker/main.go::asynq.Config.Queues` match
  exactly (same queue names, same priorities).

---

### P1.8 — DNS/CDN provider interface + registry

**Owner model**: Sonnet
**Depends on**: P1.1
**Runs in parallel with**: P1.3, P1.4, P1.5, P1.7 (after P1.2 — it doesn't
touch DB, so technically it can start right after P1.1, but parallelizing
with P1.2+ is fine)
**Reads first**: CLAUDE.md §"Provider Abstraction Layer", ADR-0002 D4 (CloneConfig idempotency), `ARCHITECTURE.md` §2.2

**Scope (in)**:
- `pkg/provider/dns/provider.go`: the `Provider` interface + `Record`
  struct + `RecordFilter` struct exactly as defined in CLAUDE.md. Do not
  add or remove methods.
- `pkg/provider/dns/registry.go`: `Register(name string, factory Factory)`,
  `GetProvider(name string) (Provider, error)`, `ListProviders() []string`.
  Thread-safe with `sync.RWMutex`.
- `pkg/provider/cdn/provider.go`: `Provider` interface + `DomainConfig`,
  `DomainStatus` structs exactly as defined in CLAUDE.md.
- `pkg/provider/cdn/registry.go`: same pattern as DNS registry.
- `pkg/provider/cdn/contract_test.go`: **contract test helper**
  `TestCloneConfig_Idempotent(t *testing.T, p cdn.Provider)` that any
  concrete provider can invoke. Per ADR-0002 D4, it must verify:
  1. Calling `CloneConfig(src, dst)` when `dst` does not exist → success,
     `dst` now mirrors `src`.
  2. Calling `CloneConfig(src, dst)` again when `dst` already exists and
     matches `src` → success, no change.
  3. Calling `CloneConfig(src, dst)` when `dst` exists but differs from
     `src` → either (a) reconciles to match `src`, or (b) returns a
     specific `ErrCloneConflict`. Document which behavior is required.
  The helper requires a `cdn.Provider` implementation, so Phase 1 cannot
  run it green against a real provider. Write it against a
  `fakeCDNProvider` defined in the same file so CI proves the helper
  itself compiles and passes.
- `pkg/provider/dns/contract_test.go`: a much smaller helper
  `TestCreateRecord_RoundTrip(t, p)` that exercises create → list → delete.
  Same "fake provider" approach.

**Scope (out)**:
- **No concrete providers** (cloudflare.go, aliyun.go, tencent.go, etc.).
  Those are Phase 2. Even if CLAUDE.md shows provider examples, Phase 1
  ships only the interface + registry + contract tests + fake.
- No real API calls, no credential handling, no retry logic beyond what
  `context.Context` provides.
- No `internal/` code calls `dns.GetProvider` in Phase 1 — subdomains are
  created but not deployed.

**Deliverables**:
- `pkg/provider/dns/provider.go`, `pkg/provider/dns/registry.go`,
  `pkg/provider/dns/contract_test.go` (with fake provider)
- `pkg/provider/cdn/provider.go`, `pkg/provider/cdn/registry.go`,
  `pkg/provider/cdn/contract_test.go` (with fake provider +
  `TestCloneConfig_Idempotent` helper)

**Acceptance**:
- `go build ./pkg/provider/...` succeeds.
- `go test ./pkg/provider/...` passes — the contract test helpers run
  against the in-file fake provider.
- The interface method signatures match CLAUDE.md exactly
  (byte-for-byte).
- `ListProviders()` returns an empty slice in Phase 1 (no concrete
  providers registered).

---

### P1.9 — Frontend: login + project/domain list views

**Owner model**: Sonnet
**Depends on**: P1.3 (login API), P1.6 (domain/project APIs)
**Reads first**: `docs/FRONTEND_GUIDE.md`, CLAUDE.md §"Frontend Conventions", existing `web/src/` code (especially the completed login page)

**Scope (in)**:
- Login flow integration:
  - Wire the existing login page to `POST /api/v1/auth/login`.
  - Store JWT in Pinia auth store + localStorage.
  - Set up an axios/fetch interceptor that attaches `Authorization: Bearer`
    to every request and redirects to `/login` on 401.
- Project list view:
  - `web/src/views/projects/ProjectList.vue`
  - `web/src/api/project.ts`
  - `web/src/types/project.ts` (mirror Go DTOs)
  - Naive UI `NDataTable` with columns: name, slug, description,
    created_at, actions.
- Project detail view (read-only for P1.9, no edit modal):
  - `web/src/views/projects/ProjectDetail.vue` showing project metadata
    and its prefix rules.
- Main domain list view:
  - `web/src/views/domains/DomainList.vue`
  - `web/src/api/domain.ts`
  - `web/src/types/domain.ts`
  - Filter by project. Show domain, status badge, subdomain count,
    created_at.
- Router + nav:
  - Update `web/src/router/index.ts` with routes for `/projects`,
    `/projects/:id`, `/domains`.
  - Update the main layout nav to include these pages.
  - Guard routes with the auth store.
- Reuse the existing light theme + brand components committed in
  `59fa29e` and `778892f`. Do **not** introduce a new design system.

**Scope (out)**:
- No create/edit modals for domains or prefix rules (Phase 2 or separate
  P1.9.x if needed).
- No status-transition UI (no "deploy", "suspend", "switch" buttons) —
  those backend endpoints don't exist in Phase 1.
- No real-time updates (no WebSocket, no polling).
- No i18n infrastructure beyond what's already scaffolded.

**Deliverables**:
- `web/src/views/projects/ProjectList.vue`, `ProjectDetail.vue`
- `web/src/views/domains/DomainList.vue`
- `web/src/api/project.ts`, `web/src/api/domain.ts`
- `web/src/types/project.ts`, `web/src/types/domain.ts`
- `web/src/stores/auth.ts` (finalize), `web/src/stores/project.ts`,
  `web/src/stores/domain.ts`
- `web/src/utils/http.ts` with auth interceptor
- Router updates

**Acceptance**:
- `npm run build` in `web/` succeeds without warnings.
- Manual smoke test: log in, land on `/projects`, see the list, click a
  project, see its detail page, navigate to `/domains`, see the list.
- 401 from the API redirects back to `/login`.
- No console errors in the browser.
- TypeScript types match the Go DTOs returned by P1.3 and P1.6
  byte-for-byte.

---

## Cross-cutting reminders

1. **CLAUDE.md rules are load-bearing.** Re-read them before any task, not
   just the first one. Critical Rule #8 (single status write path) and
   Critical Rule #9 (prefix_rules soft-freeze) in particular.

2. **Pre-launch migration exception.** `migrations/000001_init.up.sql` can
   be edited in place during Phase 1. After cutover, never again.

3. **One write path for `main_domains.status`.** The CI gate
   (`make check-status-writes`) must stay green after every task. Any PR
   that breaks it must be fixed, not merged with an exception.

4. **Phase 1 never touches real infrastructure.** If a task tempts you to
   call a provider API, render a template, commit to SVN, run a probe, or
   send a notification — STOP. That's Phase 2. Enqueue a task type with a
   stub handler instead.

5. **Log levels**: use `Info` for normal ops, `Warn` for recoverable issues,
   `Error` for failures needing attention. No `fmt.Println` in production
   code.

6. **Every API response follows the envelope** from CLAUDE.md §"Response
   Format" — `{code, data, message}`. No ad-hoc response shapes.

7. **Every multi-table write uses a transaction** (`BeginTxx` + defer
   `Rollback` + explicit `Commit`).

8. **Every external call has a context timeout.** 5s for DB, 30s for
   provider (not used in P1), 120s for SVN (not used in P1), 3s for probe
   (not used in P1).

---

## When Phase 1 is "done"

All nine cards are merged to `main`, and the following command output is
clean:

```bash
make build            # all 4 binaries compile
make lint             # golangci-lint + eslint clean
make test             # go test ./... green
make check-status-writes   # exactly one status write site
cd web && npm run build    # frontend builds
cd web && npm run lint     # frontend lints
```

Plus the manual smoke test in P1.9's acceptance criteria passes.

At that point, the project has a running control plane — operators can log
in, define projects with prefix rules, register domains, see them in the
UI, and the state machine is ready to accept transitions from Phase 2's
executors. The architecture is locked; Phase 2 plugs executors into
well-defined seams without re-touching core contracts.

---

## References

- `CLAUDE.md` — coding standards, critical rules, state machine, project layout
- `docs/ARCHITECTURE.md` — subsystem responsibilities, queue layout (§2.2)
- `docs/DATABASE_SCHEMA.md` — every table, every constraint
- `docs/DEVELOPMENT_PLAYBOOK.md` — how to add endpoints / providers / tasks / pages
- `docs/FRONTEND_GUIDE.md` — frontend conventions
- `docs/CLAUDE_CODE_INSTRUCTIONS.md` — Claude Code session protocol
- `docs/adr/0001-architecture-revision-2026-04.md` — parent architecture ADR
- `docs/adr/0002-pre-implementation-adjustments-2026-04.md` — D1–D6, E1–E3
