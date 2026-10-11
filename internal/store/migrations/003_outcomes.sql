CREATE TABLE IF NOT EXISTS lead_outcomes (
    id bigserial PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    external_id text NOT NULL,
    status text NOT NULL CHECK(status IN ('SUBMITTED','QUALIFIED','SPAM','LOST')),
    source text NOT NULL CHECK(source IN ('SERVER','MANUAL')),
    updated_by uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id, external_id)
);
CREATE INDEX IF NOT EXISTS lead_outcomes_session_idx ON lead_outcomes(session_id);
CREATE INDEX IF NOT EXISTS lead_outcomes_quality_idx ON lead_outcomes(project_id, status, updated_at DESC);
