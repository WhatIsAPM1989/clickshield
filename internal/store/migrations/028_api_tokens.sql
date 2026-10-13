CREATE TABLE api_tokens (
    id uuid PRIMARY KEY,
    name varchar(120) NOT NULL,
    project_id uuid REFERENCES projects(id) ON DELETE CASCADE,
    scopes text[] NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    prefix text NOT NULL,
    created_by uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

-- Existing analytics keys keep working as project tokens with analytics:read.
INSERT INTO api_tokens(id,name,project_id,scopes,token_hash,prefix,expires_at,revoked_at,created_at,last_used_at)
SELECT k.id,k.name,k.project_id,ARRAY['analytics:read'],k.token_hash,k.prefix,k.expires_at,k.revoked_at,k.created_at,k.last_used_at
FROM analytics.analytics_api_keys k JOIN projects p ON p.id=k.project_id;

DROP TABLE analytics.analytics_api_keys;
