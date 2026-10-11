CREATE TABLE metrika_read_connections (
 project_id uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
 counter_id text NOT NULL,
 token_encrypted bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
