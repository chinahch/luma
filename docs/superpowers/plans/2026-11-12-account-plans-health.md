# Account plans, persistent subscriptions and health implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Tests must demonstrate RED then GREEN before completion.

**Goal:** Deliver all approved account/subscription/invitation/plan/monthly quota/list-toggle/proxy-detection features in both public repositories.
**Architecture:** Reuse Go/Vue and native node, migration5 for business state, migration6 for short-lived encrypted probes. Independent non-overlapping implementation domains run in parallel; root integrates, tests and publishes, one fresh whole-change review at end.
**Tech Stack:** existing Gin/pgx/PostgreSQL, Vue/Naive UI, native Xray/sing-box.
**Spec:** ../specs/2026-11-12-account-plans-health-design.md (approved; public invitation registration confirmed).

## Global constraints
- Preserve immutable traffic reports/entries/ledger, current credential security, existing deployment identities and backward-compatible omission semantics.
- Admin and normal users both require valid active quota/expiry and node plan membership; unrestricted nodes remain visible to all eligible accounts.
- Calendar resets use configured TZ; month-end clamps and resumes anchor next month. New invited accounts have zero quota until assigned a plan. Manual order allocation only, no payment gateway.
- Real proxy tests originate on the assigned native machine, bounded and authenticated. No arbitrary commands, targets or secret output. Source and check time shown.
- Existing native installs keep original config/credentials; changed release gets a new version, never overwrite existing releases.

## Shared contracts
- Root owns server.go route wiring and auth/subscription/connectors plus new account_actions.go, invitations.go; account worker owns store migration5/store+reports and admin.go/node.go plus entitlements.go/orders.go.
- Business fields: users.subscription_cipher bytea, invite_code text unique nullable, inviter_id bigint FK users, deleted_at timestamptz. Additional calendar columns determined by business worker. node_plans(node_id,plan_id) PK, both references. Nodes list/detail expose plan_ids array, omitted updates preserve; [] removes restriction.
- Account helpers: a.nodeAllowed(ctx context.Context,userID,nodeID int64)(bool,error), a.registerBusinessRoutes(r *gin.Engine,admin *gin.RouterGroup); Store.RefreshCycles(ctx,now time.Time) error and Store.ResetUserUsage(ctx,userID,actorID int64) error. Root hooks cycle refresh before auth/subscription/nodeAuth. Business worker may refine signatures but must notify root.
- GET /api/me/subscription, GET /api/admin/users/:id/subscription -> {path,needs_recovery}; GET creates only when no existing hash; POST /api/me/subscription/recover {token}; existing POST /api/me/subscription-token explicitly rotates. POST /api/admin/users/:id/identity resets UUID+URL. POST .../:id/reset-usage; DELETE .../:id soft deletes with self/last-admin guard.
- GET /api/me/invite -> {path,code,invited_count,invites:[]}; GET /api/invites/:code -> {valid:true}; POST /api/register {email,password,invite_code}; GET /api/admin/users/:id/invites -> [] (id,email,created_at). Registered normal users use existing login.
- GET /api/admin/users/:id/orders -> []; POST same {plan_id,request_id}; order snapshots plan data and actor, idempotent request ID. GET /api/me/orders -> []. PATCH /api/admin/nodes/:id/active {active:bool} changes only state.
- Probe worker owns new httpapi/probes*.go + migration6.sql and all Caddy changes, root wires a.registerProbeRoutes(r,admin), migration6 and optional startup hooks. GET /api/me/nodes -> authorized node status list; POST /api/me/nodes/:id/probes {}; POST /api/admin/exits/:id/probes {machine_id,kernel_type}; GET /api/me/probes/:id and /api/admin/probes/:id -> task result. Result JSON fields id,state,latency_ms,error_code,created_at,finished_at,source; states queued,running,succeeded,failed,unsupported,expired.
- Native GET /api/v3/server/probes (machine auth), POST /api/v3/server/probes/:id/result (machine auth, lease binding). Machine jobs use structured outbound envelope, chosen kernel, fixed allowed target and short expiry. API job payload encrypted at rest, results sanitized. User probe uses actual user credentials and node inbound/public address, not merely its exit.
- Frontend owns only web/src, integrates those APIs. Admin menu all screenshot actions; invites/orders user pages, register invite landing, preserved subscription display/recovery, plan multiselect, list toggle, exit-test actions and node health page. HTTP clipboard fallback supported.

## Review focus
1. Stable GET must not silently invalidate old hash-only subscriptions or cross-account leak; concurrent first creation and explicit revoke tested.
2. Month reset/manual reset/late and duplicate reports/order retry concurrency retains raw ledger and exactly current-cycle charge.
3. Entitlement filter identical across native users, subscriptions, MTProxy and probe paths; omitted plan updates preserved.
4. Public invite throttling/deleted inviter/duplicate email, last-admin/self deletion and session revocation.
5. Probe lease replay, cross-machine/auth, expired tasks, error secrecy and temporary native resources; zero-to-one-user startup.

### Task 1 — Business data and entitlements (dedicated worker)
- [ ] RED: PG tests month end/leap/calendar/offline catchup/manual reset+late report+duplicate, admin/plan visibility and atomic node saves, order idempotency.
- [ ] Add migration5, cycle/accounting code; plan selectors and active-only PATCH; transactional manual orders; expose shared helpers.
- [ ] GREEN: worker tests + existing store/httpapi tests. Record commands and findings; no git commit by worker.

### Task 2 — Account/invitation/subscription (root)
- [ ] RED: stable concurrent link, old-link recovery/revocation/identity, public invitation/account privacy/rate limit, delete protections and preserved billing.
- [ ] Implement encrypted link read/recover/rotate, invitation registration+records, user actions; integrate business/probe route hooks and migration6.
- [ ] GREEN: full real PG backend and race in Linux CI.

### Task 3 — Native proxy probes (dedicated worker)
- [ ] RED: actual local proxy success/auth failure/timeout, job isolation/expiry, and zero-to-first-user startup.
- [ ] Implement bounded native probe worker and authenticated encrypted API tasks; bump Caddy v0.1.3-preview, add release/CI tests.
- [ ] GREEN: new tests, complete native internal/CLI with 29 fixtures and shell/Python guards.

### Task 4 — User/admin UI (dedicated worker)
- [ ] RED: meaningful serialization/state helper tests for durable links, plan selection, status/toggle rollback and invitation validation where applicable.
- [ ] Implement all approved flows; no fabricated data or unsupported-success labels.
- [ ] GREEN: node tests, vue-tsc and production build; root real desktop/mobile browser QA.

### Task 5 — Review, publication and delivery (root)
- [ ] Integrate and verify real DB, full affected native, UI and browser workflows; record decisions.
- [ ] One fresh whole-change reviewer; required findings fixed with RED→GREEN regressions and full affected checks, no redundant second review.
- [ ] Publish exact tested source trees to feature PRs; follow CI, merge/publish native v0.1.3-preview, verify public packages, pin installer then merge panel; main CI green.
- [ ] Deliver upgrade commands, migration/old-link limits, native existing-host upgrade, verification evidence. Stop owned local previews, retain data; no production login or changes.

## Completion ledger

Tasks1–4 implementation complete, real PG full tests/vet and frontend22 tests/type/build green. One fresh whole-change review found4 Important+1 Minor, all required by operational effect and all reproduced RED→GREEN: shared Xray creation lock, preserved global logging, connectivity-only result selection, encrypted H3 transport inspection, recent physical evidence only for autofill. No deferred findings or second review. User subsequently requested ICMP latency (not HTTP duration); native/PG/UI tests exercise ordering and separate RTT failure semantics, and Linux release CI requires real privileged ICMP loopback.

Rulings: keep manually entered hosts on machine changes; no persisted automatic-host provenance. NAT/hairpin/firewall/physical host proofs remain target-host acceptance; missing evidence is displayed, never guessed. Subscription old hash-only cannot be reconstructed; explicit recovery/rotation is documented. Publication and actual public-download checks remain pending until corresponding Actions succeed. Installer pinned to Caddy bd0a796db6a43129dfdbce9adfe3336f77e47569 / v013.
