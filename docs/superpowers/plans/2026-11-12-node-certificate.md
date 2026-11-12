# 节点证书自动下发 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 管理员通过 XB 风格表单管理 TLS 证书，原生节点自动接收配置、申请与续期。

**Architecture:** Luma 提供证书策略校验、凭据保留与安全配置接口；Vue 证书组件独立于协议密钥生成。Caddy 复用 CertMagic/libdns，修复必要的生命周期缺陷，使用本地签发器和存储验证证书与内核更新，不调用公共 CA 测试。

**Tech Stack:** Go, Vue 3/Naive UI, PostgreSQL, CertMagic/libdns, native caddy, existing Docker Compose panel.

**Spec:** docs/superpowers/specs/2026-11-12-node-certificate-design.md（用户 2026-11-12 确认按方案实施）。

## Global Constraints

- 节点为原生二进制，不需要 Docker；默认 DNS-01，兼容文件/HTTP-01/PEM，不自动降级自签名。
- 凭据加密保存；订阅、普通节点列表、诊断结果不含 DNS 密钥/PEM 私钥。
- 保存后沿用当前受认证的配置同步；区分已保存、等待申请与已签发。
- 证书域名、具体 SNI、订阅 public_host 不混淆；同机多节点证书目录隔离。
- 仅使用本地可控组件进行测试；不使用用户截图凭据，不对公共 CA 发测试订单。

## Review Focus

- 切换提供商后留空不能沿用旧厂商密钥；对应 Task 1 API 回归。
- 编辑现有文件配置、取消编辑或仅换证书不能重生协议密钥；对应 Task 2 状态回归。
- 失败后相同配置再次同步仍可重试；对应 Task 3 生命周期回归。
- 节点删除/配置变更应停旧续期任务；对应 Task 3 cache stop 回归。
- ICMP/代理探测及用户订阅不得获取 DNS 凭据；对应 Task 1 接口隔离回归。

### Task 1: 证书策略、校验和受限凭据接口

**Files:** create internal/protocol/certificate.go, certificate_test.go; modify internal/protocol/preset.go; modify internal/httpapi/protocols.go, admin.go, http.go (route owner as found); create internal/httpapi/certificates.go, certificates_test.go.

**Interfaces:** protocol.CertificateConfig maps exact native cert_config keys; DNSProviders() returns canonical provider metadata/credential groups from pinned registry; ValidateCertificate(config map[string]any) error validates raw configuration. GET /api/admin/certificates/providers returns metadata; GET node detail redacts dns_env/key_content with saved indicators; cert_secret_action=keep/replace applied before validation to preserve existing credentials only when mode/provider match.

- [ ] Write and run failing tests for DNS defaults, Cloudflare aliases, first-equals parsing, provider credentials and SNI coverage; `go test ./internal/protocol -count=1` must fail on missing feature.
- [ ] Implement schema/provider catalogue/validation and extend preset input while preserving legacy cert_file/key_file callers.
- [ ] Write real PG API failing tests: encrypted roundtrip to assigned machine, masked admin response, keep/replace/mode-change rules, invalid input atomic rollback, subscriptions/probes without DNS credentials.
- [ ] Implement node certificate sanitization/credential retention within existing node transaction, routes and errors; run protocol and httpapi tests. Commit Task 1.

### Task 2: XB 风格证书表单

**Files:** create web/src/certificates.ts, certificates.test.ts, components/NodeCertificate.vue; modify components/ProtocolPreset.vue, Resources.vue.

**Interfaces:** CertificateDraft/fromConfig/applyCertificateToConfig operate on copies; NodeCertificate consumes full public node config, secret saved indicators and provider metadata, emits merged config. Existing preset emit includes public certificate config without DNS secret echoes. Client keep/replace flags consumed by Task 1.

- [ ] Write/run failing Node tests for hydration of legacy file, defaults for TLS, mode switching, CRLF KEY=VALUE split, duplicate rejection, preserving protocol keys and stale secret removal.
- [ ] Implement pure form state and certificate component with DNS/HTTP/file/content modes, provider selector/help, masked credential input, saved indicators, domain/email/SNI validation messages.
- [ ] Wire component into new/edit node form separately from preset generation; remove mandatory manual-file warning. Save message says waiting for sync/issue. Run `pnpm test`, `pnpm check`, `pnpm build`. Commit Task 2.

### Task 3: 原生证书生命周期与状态

**Files:** create outputs/caddy/scripts/apply_certificates.py, patches/certificates.go or targeted source patch helpers, tests/certificate_*_test.go; modify scripts/prepare_source.py as needed; native targeted files internal/cert/cert.go, internal/service/service.go, internal/machine status projection through reproducible patch.

**Interfaces:** Certificate status is fixed enum plus non-secret domain/mode/expiry/error-code; only actual material valid for SNI permits TLS listener. All certificate work is instance-local and cancellation safe. Native code uses existing source model CertificateConfig; no global environment mutation or shell.

- [ ] Add fail-first tests proving cached certificate reuse across restart, unchanged refresh, failed apply retry, changed-domain stale cert exclusion, Stop cancels renewal/cache workers, renewal applied to correct kernel and multi-node storage separation.
- [ ] Fix only defects demonstrated by tests, use injectable local issuer/storage/clock as appropriate with production public-CA defaults fixed. Mask library errors to fixed codes in outgoing status/logs.
- [ ] Add API/native delivery integration using full cert_config from Task 1; ensure diagnostics reflect pending/failure without false connectivity.
- [ ] Prepare reproducible source, run cert/service/machine tests plus full tagged native suite; build Linux amd64/arm64 and run release archive tests. Commit Task 3.

### Task 4: 集成、审阅、发布和部署说明

**Files:** docs/node-certificates.md; native docs/certificates.md; scripts/release/build metadata/version pins as required; durable outputs/节点证书升级进度.md.

- [ ] Run whole Luma Go+PG, frontend checks, native tagged suite, installer/archive checks once final code is stable. Record exact commands/results.
- [ ] Browser check new/edit forms, secret preservation/cancel, provider switch validation and pending/failed messages. Never issue real public CA orders from QA.
- [ ] One fresh whole-change reviewer checks spec, plan and diff; classify findings, fix required findings RED→GREEN and rerun affected whole suites. No per-task reviewer or duplicate review.
- [ ] Publish Caddy native preview only if changed, verify public package/checksum, then pin Luma installer to tested Caddy commit/release; publish and merge authorized repository changes with CI green.
- [ ] Deliver panel update command and native update instructions if necessary; never claim production machines already updated.

### Task 5: 用户追加的自动自测与服务器卡片（发布前实施）

**Files:** internal/store/migration7.sql, store.go; internal/httpapi/automatic_health.go, probes.go, server_cards.go; web/src/serverCards.ts, components/NodeHealth.vue, ServerNodeCards.vue, Resources.vue.

**Interfaces:** Config.AutomaticHealth enabled by production main; authenticated machine probe poll calls scheduleMachineHealth under existing machine row lock. proxy_probes.automatic distinguishes shared server tests; nullable user_id only used for unsupported no-user records, actual queued tasks always use eligible user. GET /api/admin/machines/layout and PUT with machine_ids persist per-admin order; country source is fixed explicit-IP Country.is with cached validated results.

- [ ] RED→GREEN real PG: native poll without browser schedules exactly once/5min; correct entitlement and machine checks retained; ordinary users see only allowed node status.
- [ ] RED→GREEN layout: preserve order across GET, reject duplicate/unknown IDs; pure UI grouping includes empty servers and unassigned nodes; move operation and unknown country behavior.
- [ ] Geo tests: fake HTTP transport verifies explicit physical IP endpoint, response IP equality, cache, no WARP source; no production API calls in automated tests.
- [ ] NodeHealth removes manual triggers and refreshes every15s, marks results older than10min stale; Resources provides server cards/draggable persisted order and current-machine default on add.
- [ ] Test/build and include in the single whole-branch review and final publication.
