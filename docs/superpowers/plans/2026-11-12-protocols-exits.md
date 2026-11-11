# Protocols and landing implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans. User approved continuation and the concrete protocol/authentication list; continue without repeating design questions.

**Goal:** Add typed protocol presets and locally parsed proxy-link landing configurations to Luma, with compatible node generation and explicit connector support.

**Architecture:** Preserve the audited pinned node; add typed Xray stream settings and correct sing-box detour mapping. Compile landing proxy profiles into kernel-specific outbounds and ordered rules. Use a companion controlled connector worker for Cloudflare/MTProxy rather than pretending they are native inbounds.

**Tech Stack:** Existing Go/Vue/PostgreSQL stack; AES-256-GCM for reusable proxy credentials; pinned source/kernel validation.

**Spec:** ../specs/2026-11-12-open-source-design.md and ../../protocol-roadmap.md.

## Global constraints
- No arbitrary URL fetch on paste. Parse single/multiline supported URI forms locally; errors identify line numbers without returning credentials in logs.
- X25519/ML-KEM-768 authentication × native/xorpub/random appearance are six distinct explicit choices; validate against the pinned fork.
- Native presets cover SOCKS5, VLESS Reality/ENC/WS/XHTTP, VMess WS, SS/SS2022, Hysteria2 and AnyTLS TLS/Reality. Reject unsupported kernel combinations.
- Cloudflare Tunnel needs a connector and supports its actual service routing, not arbitrary UDP. MTProxy needs attributable per-user instances if billing is enabled; shared aggregate metrics must not masquerade as per-user bytes.
- Existing source/public main remains a labeled preview; new feature stays on feat/protocols-exits until tests/review pass.

## Review focus
- Credentials leak in list endpoints, logs, plaintext SQL or imported links re-rendering to unauthorized users.
- URI escaping, IPv6, Base64 variants, malformed port, embedded password colon/@ and duplicate/oversized batches.
- Kernel-specific TLS/Reality, stream settings, detour and default catch-all semantics; no silent direct fallback.
- Node/Machine ownership and minimum capability checks on configuration delivery.
- Connector execution must use allowlisted binaries and argument arrays, private token files, bounded processes and real readiness signals.

## Tasks
1. **Link parser and native outbound compilation** — internal/proxy/{profile,parse,outbound}.go and tests. Parse vless/vmess/ss/socks5/http/trojan/hysteria2/anytls URI, preserve TLS/Reality/ENC/WS/XHTTP parameters, reject unsupported plugin/invalid inputs; compile both kernel dialects with meaningful unit fixtures.
2. **Encryption and database migration** — internal/secretbox, store migration v2. Add exits/routes/node private config with encrypted reusable credentials. Empty/update/preserve secret semantics explicit; master key stays in private environment and migration backups.
3. **Landing API/UI** — admin import preview/save/edit/disable endpoints, assign default or domain/CIDR routing per node, show capability conflicts; editable masked lists, no unapproved remote probing.
4. **Protocol presets and subscriptions** — typed protocol settings/keys/certificates; all native requested combinations generate validated configs plus matching client export. Test the actual pinned kernel parsers, not just map key existence.
5. **Node bridge and capabilities** — Xray stream_settings model forwarding, sing-box detour, client capability handshake; reject configurations requiring unsupported installed features.
6. **CF/MTProxy connector worker** — pinned implementations, controlled process execution, Machine/Node-scoped desired state, readiness/status, attributable statistics. External Cloudflare account connection remains environment-dependent and labeled until actually exercised.
7. **Transport integrity and delivery** — durable stable batch queue where introduced, real PostgreSQL and Linux CI checks, fresh read-only review, publish a feature PR with exact validation/remaining limits.

Each behavioral task follows RED→GREEN. Preserve the full development ledger in the task outputs; publish only source/product documentation, not local login/runtime details.
