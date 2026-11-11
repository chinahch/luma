CREATE TABLE plans (
 id bigserial PRIMARY KEY, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 quota_bytes bigint NOT NULL CHECK(quota_bytes>=0), duration_days integer NOT NULL CHECK(duration_days BETWEEN 1 AND 3650),
 price_cents bigint NOT NULL DEFAULT 0 CHECK(price_cents>=0), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE users (
 id bigserial PRIMARY KEY, email text NOT NULL UNIQUE CHECK(email=lower(email)), password_hash text NOT NULL,
 role text NOT NULL DEFAULT 'user' CHECK(role IN ('admin','user')), uuid uuid NOT NULL UNIQUE,
 subscription_hash bytea UNIQUE, active boolean NOT NULL DEFAULT true,
 plan_id bigint REFERENCES plans(id), quota_bytes bigint NOT NULL DEFAULT 0 CHECK(quota_bytes>=0),
 used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0), expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash bytea PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE login_attempts (key_hash bytea PRIMARY KEY, attempts integer NOT NULL, window_start timestamptz NOT NULL);
CREATE TABLE machines (
 id bigserial PRIMARY KEY, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100), token_hash bytea NOT NULL UNIQUE,
 last_report_at timestamptz, status jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE nodes (
 id bigserial PRIMARY KEY, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 machine_id bigint REFERENCES machines(id), token_hash bytea NOT NULL UNIQUE, active boolean NOT NULL DEFAULT true,
 multiplier_milli integer NOT NULL DEFAULT 1000 CHECK(multiplier_milli BETWEEN 1 AND 100000),
 report_interval integer NOT NULL DEFAULT 60 CHECK(report_interval BETWEEN 10 AND 3600),
 config jsonb NOT NULL DEFAULT '{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"network":"tcp","tls":0}',
 public_host text NOT NULL DEFAULT '', last_report_at timestamptz, last_period_end timestamptz, last_quality text,
 last_up_bytes bigint NOT NULL DEFAULT 0, last_down_bytes bigint NOT NULL DEFAULT 0,
 last_window_seconds double precision NOT NULL DEFAULT 60,
 status jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX nodes_machine ON nodes(machine_id);
CREATE TABLE usage_reports (
 id bigserial PRIMARY KEY, node_id bigint NOT NULL REFERENCES nodes(id), report_id text NOT NULL,
 payload_hash bytea NOT NULL, quality text NOT NULL CHECK(quality IN ('reliable','legacy')),
 period_start timestamptz NOT NULL, period_end timestamptz NOT NULL CHECK(period_end>period_start),
 multiplier_milli integer NOT NULL CHECK(multiplier_milli BETWEEN 1 AND 100000),
 received_at timestamptz NOT NULL DEFAULT now(), UNIQUE(node_id,report_id)
);
CREATE INDEX reports_event_time ON usage_reports(period_end,node_id);
CREATE TABLE usage_entries (
 report_id bigint NOT NULL REFERENCES usage_reports(id), user_id bigint NOT NULL REFERENCES users(id),
 up_bytes bigint NOT NULL CHECK(up_bytes>=0), down_bytes bigint NOT NULL CHECK(down_bytes>=0),
 billed_bytes bigint NOT NULL CHECK(billed_bytes>=0), PRIMARY KEY(report_id,user_id)
);
CREATE INDEX usage_user ON usage_entries(user_id,report_id);
CREATE TABLE billing_ledger (
 id bigserial PRIMARY KEY, report_id bigint NOT NULL REFERENCES usage_reports(id), user_id bigint NOT NULL REFERENCES users(id),
 billed_bytes bigint NOT NULL CHECK(billed_bytes>=0), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(report_id,user_id)
);
CREATE INDEX ledger_user_time ON billing_ledger(user_id,created_at);
CREATE TABLE schema_version (id integer PRIMARY KEY CHECK(id=1), version integer NOT NULL);
INSERT INTO schema_version(id,version) VALUES(1,1);
