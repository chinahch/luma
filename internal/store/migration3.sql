CREATE TABLE node_private (
 node_id bigint PRIMARY KEY REFERENCES nodes(id), encrypted_config bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
UPDATE schema_version SET version=3 WHERE id=1;
