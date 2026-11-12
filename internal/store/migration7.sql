ALTER TABLE proxy_probes ADD COLUMN automatic boolean NOT NULL DEFAULT false;
ALTER TABLE proxy_probes ALTER COLUMN user_id DROP NOT NULL;
CREATE INDEX proxy_probes_auto_node ON proxy_probes(node_id,created_at DESC) WHERE automatic;
CREATE TABLE machine_card_layouts (
 user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 machine_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE machine_countries (
 machine_id bigint PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
 ip inet NOT NULL, country text NOT NULL DEFAULT '' CHECK(country='' OR country ~ '^[A-Z]{2}$'),
 checked_at timestamptz NOT NULL DEFAULT now()
);
UPDATE schema_version SET version=7 WHERE id=1;
