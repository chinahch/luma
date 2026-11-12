# Protocol and landing validation (2026-11-12)

- Real PostgreSQL tests pass for encrypted exits, redacted lists, admin/user isolation, capability gates, disabled exits, per-user MTG discovery/quota and report replay.
- 23 Xray and 6 sing-box generated server/client pairs complete actual isolated loopback SOCKS→protocol→server→TCP-echo traffic, including six ENC combinations, AnyTLS Reality, SS2022, Hysteria2 and XHTTP H3. Certificates are local test certificates; Xray uses certificate SHA256 pins.
- Browser QA passes for multiline IPv6 landing import/dedup/edit/save/default routing and ML-KEM-768/random protocol generation and node creation. Frontend typecheck, production build and existing native tests pass.
- Worker process/lease tests pass. Official mtg v2.3.0 from the pinned Go module accepts generated TOML and serves loopback metrics. Immutable queue and retry tests preserve report identities; Telegram-only counters exclude domain fronting.
- Baseline Linux CI passed for node source/build and panel Compose/database backup→empty-instance restore. Feature Linux CI adds native data-plane, connector/vendor smoke and encrypted credential recovery; check the corresponding commit's Actions results.

Still unverified: actual CF account/hostname attachment, external Telegram client sessions, Linux systemd/OpenRC host operation, ARM64 runtime/128 MiB capacity and production load. Native node traffic still uses the documented best-effort legacy sender. Automatic node upgrade remains disabled until logical-node readiness can be established.

## Final review regression pass

Independent read-only review identified six Important defects. Each was reproduced before its fix, then verified with the full affected suites: arbitrary custom-outbound secret projection/encryption; native Xray private-key error/log leakage on Start/Reload; MTG database/route failure refusing direct leases; sustained 64-node reporting and independent status/report budgets; per-node rejected-report preservation/isolation; concurrent native/Telegram subscriptions with a one-connection PostgreSQL pool. No reviewer Minor findings were deferred.

The connector regression also stalls a real status HTTP request while durable reports continue delivering, and verifies that an identity with rejected reports does not execute a process. Mobile 390×844 landing layout was visually rechecked after stacking the heading/toolbar.

## Historical bootstrap checks


Verified on Windows amd64 with Go 1.26.10 and an isolated real PostgreSQL 16.15 test database:
- `go test -p 2 ./...`: passed, including session/RBAC, Machine boundaries, batch idempotency/conflict, transactional rollback, late-window handling, nullable expiry and VLESS field projection tests.
- Node 24 / pnpm 11.25.0: frontend typecheck and production Vite build passed; three native Node tests passed for account refresh, nullable expiry and timezone/DST calendar ranges.
- Browser QA: administrator login, plan creation and database-backed traffic view. A 40 MiB upload + 120 MiB download window at multiplier 1.5 bills 240 MiB; replaying the same reliable batch did not double debit. Desktop and mobile dark layout inspected.
- The actual pinned XBoard-Node `panel.Client` exercised handshake, Node config/users/report, Machine node discovery/config/users/status against the local Luma API and passed. This is a control-plane contract check, not a real proxy traffic connection.
- An isolated official Windows Nginx 1.28.0 runtime verified that actual HTML responses retain CSP and X-Frame-Options after the header inheritance fix. Test adaptation changed only listener/upstream/static paths and Windows log path.

The Windows bootstrap environment could not run Docker or Linux services; subsequent baseline Linux CI completed Compose/dump/restore. Current loopback data-plane results are listed above. Production load remains unverified. The CI workflow includes Linux Go tests, frontend checks, isolated Compose startup and restore into a new empty instance; its result must be checked separately.

Independent read-only review identified six Important defects. Fixes were verified by regression tests: CLI serialization (in the node repository), unsafe process-only upgrade gate (disabled), unknown Caddy service collision guard, unlimited expiry preservation, refreshed account snapshots and subscription aliases. Date shortcut and HTML header findings were regraded by effect and fixed as well.
