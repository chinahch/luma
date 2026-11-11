# Initial validation (2026-11-12)

Verified on Windows amd64 with Go 1.26.10 and an isolated real PostgreSQL 16.15 test database:
- `go test -p 2 ./...`: passed, including session/RBAC, Machine boundaries, batch idempotency/conflict, transactional rollback, late-window handling, nullable expiry and VLESS field projection tests.
- Node 24 / pnpm 11.25.0: frontend typecheck and production Vite build passed; three native Node tests passed for account refresh, nullable expiry and timezone/DST calendar ranges.
- Browser QA: administrator login, plan creation and database-backed traffic view. A 40 MiB upload + 120 MiB download window at multiplier 1.5 bills 240 MiB; replaying the same reliable batch did not double debit. Desktop and mobile dark layout inspected.
- The actual pinned XBoard-Node `panel.Client` exercised handshake, Node config/users/report, Machine node discovery/config/users/status against the local Luma API and passed. This is a control-plane contract check, not a real proxy traffic connection.
- An isolated official Windows Nginx 1.28.0 runtime verified that actual HTML responses retain CSP and X-Frame-Options after the header inheritance fix. Test adaptation changed only listener/upstream/static paths and Windows log path.

Not executed in this environment: Docker image build/Compose startup, Linux service runtime, full dump/restore rehearsal, real proxy client E2E and production load. The CI workflow includes Linux Go tests, frontend checks, isolated Compose startup and restore into a new empty instance; its result must be checked separately.

Independent read-only review identified six Important defects. Fixes were verified by regression tests: CLI serialization (in the node repository), unsafe process-only upgrade gate (disabled), unknown Caddy service collision guard, unlimited expiry preservation, refreshed account snapshots and subscription aliases. Date shortcut and HTML header findings were regraded by effect and fixed as well.
