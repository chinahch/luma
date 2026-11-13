CREATE TABLE renewal_requests (
 id bigserial PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES users(id),
 plan_id bigint NOT NULL REFERENCES plans(id),
 request_id text NOT NULL CHECK(length(request_id) BETWEEN 1 AND 128),
 plan_name text NOT NULL,
 quota_bytes bigint NOT NULL CHECK(quota_bytes>=0),
 duration_days integer NOT NULL CHECK(duration_days>0),
 price_cents bigint NOT NULL CHECK(price_cents>=0),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected','cancelled')),
 order_id bigint REFERENCES orders(id),
 actor_id bigint REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 resolved_at timestamptz,
 UNIQUE(user_id,request_id)
);
CREATE UNIQUE INDEX renewal_one_pending ON renewal_requests(user_id) WHERE status='pending';
CREATE INDEX renewal_status ON renewal_requests(status,id DESC);
UPDATE schema_version SET version=8 WHERE id=1;
