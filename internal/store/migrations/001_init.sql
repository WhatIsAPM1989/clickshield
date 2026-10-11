CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    public_key text NOT NULL UNIQUE,
    allowed_origins text[] NOT NULL DEFAULT ARRAY[]::text[],
    mode text NOT NULL DEFAULT 'MONITORING' CHECK (mode IN ('MONITORING','BLOCKING')),
    scoring_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admin_users (
    id uuid PRIMARY KEY,
    username text NOT NULL,
    password_hash text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS admin_users_username_lower_idx ON admin_users(lower(username));

CREATE TABLE IF NOT EXISTS admin_sessions (
    id uuid PRIMARY KEY,
    admin_user_id uuid NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    csrf_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_sessions_expires_idx ON admin_sessions(expires_at);

CREATE TABLE IF NOT EXISTS visitors (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    visitor_key text NOT NULL,
    fingerprint text NOT NULL DEFAULT '',
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id, visitor_key)
);
CREATE INDEX IF NOT EXISTS visitors_project_fp_idx ON visitors(project_id, fingerprint) WHERE fingerprint <> '';

CREATE TABLE IF NOT EXISTS sessions (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    visitor_id uuid NOT NULL REFERENCES visitors(id) ON DELETE CASCADE,
    visitor_key text NOT NULL,
    fingerprint text NOT NULL DEFAULT '',
    yandex_client_id text NOT NULL DEFAULT '',
    google_client_id text NOT NULL DEFAULT '',
    yclid text NOT NULL DEFAULT '',
    gclid text NOT NULL DEFAULT '',
    utm_source text NOT NULL DEFAULT '',
    utm_medium text NOT NULL DEFAULT '',
    utm_campaign text NOT NULL DEFAULT '',
    utm_term text NOT NULL DEFAULT '',
    utm_content text NOT NULL DEFAULT '',
    referrer text NOT NULL DEFAULT '',
    landing_url text NOT NULL DEFAULT '',
    last_page_url text NOT NULL DEFAULT '',
    ip inet,
    asn text NOT NULL DEFAULT '',
    as_org text NOT NULL DEFAULT '',
    country text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    device_type text NOT NULL DEFAULT 'unknown' CHECK (device_type IN ('desktop','mobile','tablet','unknown')),
    device jsonb NOT NULL DEFAULT '{}'::jsonb,
    automation jsonb NOT NULL DEFAULT '{}'::jsonb,
    page_count integer NOT NULL DEFAULT 1,
    started_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    finish_requested_at timestamptz,
    finished_at timestamptz,
    current_classification_id bigint,
    current_classification text NOT NULL DEFAULT '',
    current_risk_score integer NOT NULL DEFAULT 0 CHECK(current_risk_score BETWEEN 0 AND 100),
    manual_label text NOT NULL DEFAULT '' CHECK(manual_label IN ('','HUMAN','BOT')),
    manual_note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_project_started_idx ON sessions(project_id, started_at DESC);
CREATE INDEX IF NOT EXISTS sessions_project_class_idx ON sessions(project_id, current_classification, started_at DESC);
CREATE INDEX IF NOT EXISTS sessions_project_ip_idx ON sessions(project_id, ip, started_at DESC) WHERE ip IS NOT NULL;
CREATE INDEX IF NOT EXISTS sessions_project_fp_idx ON sessions(project_id, fingerprint, started_at DESC) WHERE fingerprint <> '';
CREATE INDEX IF NOT EXISTS sessions_finish_due_idx ON sessions(finish_requested_at) WHERE finish_requested_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS sessions_stale_idx ON sessions(last_seen_at) WHERE finished_at IS NULL;

CREATE TABLE IF NOT EXISTS raw_events (
    id bigserial PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    batch_id uuid NOT NULL,
    event_index integer NOT NULL,
    event_time timestamptz NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id,batch_id,event_index)
);
CREATE INDEX IF NOT EXISTS raw_events_session_time_idx ON raw_events(session_id,event_time,id);
CREATE INDEX IF NOT EXISTS raw_events_created_idx ON raw_events(created_at);

CREATE TABLE IF NOT EXISTS session_features (
    session_id uuid PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    duration_ms bigint NOT NULL DEFAULT 0,
    active_time_ms bigint NOT NULL DEFAULT 0,
    idle_time_ms bigint NOT NULL DEFAULT 0,
    mouse_distance double precision NOT NULL DEFAULT 0,
    mouse_velocity double precision NOT NULL DEFAULT 0,
    mouse_max_velocity double precision NOT NULL DEFAULT 0,
    mouse_acceleration double precision NOT NULL DEFAULT 0,
    mouse_curvature double precision NOT NULL DEFAULT 0,
    mouse_pauses integer NOT NULL DEFAULT 0,
    mouse_moves integer NOT NULL DEFAULT 0,
    scroll_depth double precision NOT NULL DEFAULT 0,
    scroll_velocity double precision NOT NULL DEFAULT 0,
    scroll_reversals integer NOT NULL DEFAULT 0,
    scroll_events integer NOT NULL DEFAULT 0,
    click_count integer NOT NULL DEFAULT 0,
    click_intervals_ms jsonb NOT NULL DEFAULT '[]'::jsonb,
    mean_click_interval_ms double precision NOT NULL DEFAULT 0,
    time_to_first_click_ms bigint NOT NULL DEFAULT -1,
    page_count integer NOT NULL DEFAULT 1,
    interaction_density double precision NOT NULL DEFAULT 0,
    active_ratio double precision NOT NULL DEFAULT 0,
    paid_visit_frequency integer NOT NULL DEFAULT 0,
    sessions_per_ip integer NOT NULL DEFAULT 0,
    sessions_per_fingerprint integer NOT NULL DEFAULT 0,
    computed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS classifications (
    id bigserial PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    risk_score integer NOT NULL CHECK(risk_score BETWEEN 0 AND 100),
    classification text NOT NULL CHECK(classification IN ('HUMAN','SUSPICIOUS','BOT','KNOWN_BOT')),
    source text NOT NULL CHECK(source IN ('AUTO','MANUAL')),
    created_by uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS classifications_session_idx ON classifications(session_id,created_at DESC,id DESC);

CREATE TABLE IF NOT EXISTS classification_reasons (
    id bigserial PRIMARY KEY,
    classification_id bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE,
    code text NOT NULL,
    weight integer NOT NULL DEFAULT 0,
    details jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS classification_reasons_class_idx ON classification_reasons(classification_id);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='sessions_current_classification_fk') THEN
        ALTER TABLE sessions ADD CONSTRAINT sessions_current_classification_fk
            FOREIGN KEY(current_classification_id) REFERENCES classifications(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS integrations (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK(provider IN ('YANDEX_METRICA','GOOGLE_ANALYTICS')),
    enabled boolean NOT NULL DEFAULT false,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_encrypted bytea,
    last_status text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    last_sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id,provider)
);

CREATE TABLE IF NOT EXISTS integration_outbox (
    id bigserial PRIMARY KEY,
    integration_id uuid NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    classification_id bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE,
    provider text NOT NULL,
    event_name text NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','SENDING','RETRY','SENT','SKIPPED','FAILED')),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz,
    UNIQUE(session_id,provider,event_name)
);
CREATE INDEX IF NOT EXISTS integration_outbox_due_idx ON integration_outbox(status,next_attempt_at,id) WHERE status IN ('PENDING','RETRY');

CREATE TABLE IF NOT EXISTS audit_log (
    id bigserial PRIMARY KEY,
    admin_user_id uuid REFERENCES admin_users(id) ON DELETE SET NULL,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_log_created_idx ON audit_log(created_at DESC);
