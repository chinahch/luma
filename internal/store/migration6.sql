CREATE TABLE machine_probe_capabilities (
 machine_id bigint PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
 last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE proxy_probes (
 id uuid PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 node_id bigint REFERENCES nodes(id), machine_id bigint NOT NULL REFERENCES machines(id),
 kind text NOT NULL CHECK(kind IN ('node','exit','diagnostic')),
 state text NOT NULL CHECK(state IN ('queued','running','succeeded','failed','unsupported','expired')),
 encrypted_payload bytea, lease_hash bytea, diagnostics jsonb,
 ping_ms double precision CHECK(ping_ms>=0 AND ping_ms<=15000), ping_error_code text NOT NULL DEFAULT '',
 latency_ms integer CHECK(latency_ms BETWEEN 0 AND 15000), error_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
 finished_at timestamptz, source text NOT NULL DEFAULT 'node machine'
);
CREATE INDEX proxy_probes_queue ON proxy_probes(machine_id,created_at) WHERE state IN ('queued','running');
CREATE INDEX proxy_probes_machine ON proxy_probes(machine_id,created_at DESC);
CREATE INDEX proxy_probes_user ON proxy_probes(user_id,created_at DESC);
UPDATE schema_version SET version=6 WHERE id=1;
