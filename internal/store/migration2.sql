CREATE TABLE exits (
 id bigserial PRIMARY KEY, secret_id uuid NOT NULL UNIQUE,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100), protocol text NOT NULL,
 server text NOT NULL, port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
 active boolean NOT NULL DEFAULT true, encrypted_profile bytea NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE node_exit_routes (
 node_id bigint PRIMARY KEY REFERENCES nodes(id),
 config jsonb NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
UPDATE schema_version SET version=2 WHERE id=1;
