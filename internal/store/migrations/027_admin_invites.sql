CREATE TABLE IF NOT EXISTS admin_invites (
    id uuid PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE,
    created_by uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    used_by uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
