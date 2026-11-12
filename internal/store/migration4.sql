CREATE TABLE machine_enrollments (
 machine_id bigint PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
 code_hash bytea NOT NULL UNIQUE,
 claimed_token_hash bytea,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
UPDATE schema_version SET version=4 WHERE id=1;
