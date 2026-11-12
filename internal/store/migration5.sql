ALTER TABLE users ADD COLUMN subscription_cipher bytea,
 ADD COLUMN invite_code text UNIQUE,
 ADD COLUMN inviter_id bigint REFERENCES users(id),
 ADD COLUMN deleted_at timestamptz,
 ADD COLUMN billing_anchor_at timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN cycle_started_at timestamptz NOT NULL DEFAULT '-infinity',
 ADD COLUMN next_reset_at timestamptz;
UPDATE users SET billing_anchor_at=created_at;
ALTER TABLE nodes ADD COLUMN deleted_at timestamptz;
CREATE TABLE node_plans (
 node_id bigint NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
 plan_id bigint NOT NULL REFERENCES plans(id),
 PRIMARY KEY(node_id,plan_id)
);
CREATE TABLE usage_resets (
 id bigserial PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 actor_id bigint REFERENCES users(id), old_used_bytes bigint NOT NULL CHECK(old_used_bytes>=0),
 reason text NOT NULL CHECK(reason IN ('manual','calendar','order')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE orders (
 id bigserial PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 actor_id bigint NOT NULL REFERENCES users(id), plan_id bigint NOT NULL REFERENCES plans(id),
 request_id text NOT NULL CHECK(length(request_id) BETWEEN 1 AND 128),
 plan_name text NOT NULL, quota_bytes bigint NOT NULL CHECK(quota_bytes>=0),
 duration_days integer NOT NULL, price_cents bigint NOT NULL CHECK(price_cents>=0),
 status text NOT NULL DEFAULT 'allocated' CHECK(status='allocated'),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,request_id)
);
CREATE INDEX orders_user ON orders(user_id,id);
UPDATE schema_version SET version=5 WHERE id=1;
