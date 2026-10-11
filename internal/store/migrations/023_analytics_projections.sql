CREATE SCHEMA analytics;

-- This reporting model has no foreign keys or row types from the antifraud schema.
CREATE TABLE analytics.sessions (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    visitor_id uuid NOT NULL,
    started_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    traffic_source text NOT NULL,
    traffic_channel text NOT NULL,
    referrer_host text NOT NULL,
    attribution_basis text NOT NULL,
    landing_url text NOT NULL,
    last_page_url text NOT NULL,
    utm_source text NOT NULL,
    utm_medium text NOT NULL,
    utm_campaign text NOT NULL,
    utm_content text NOT NULL,
    utm_term text NOT NULL,
    device_type text NOT NULL,
    device jsonb NOT NULL,
    user_agent text NOT NULL,
    country text NOT NULL,
    region text NOT NULL,
    city text NOT NULL,
    asn text NOT NULL,
    as_org text NOT NULL,
    browser text NOT NULL,
    os text NOT NULL,
    page_count integer NOT NULL,
    current_classification text NOT NULL,
    current_risk_score integer NOT NULL,
    bot_name text,
    bot_category text NOT NULL
);
CREATE INDEX analytics_sessions_project_idx ON analytics.sessions(project_id,started_at DESC);
CREATE INDEX analytics_sessions_visitor_idx ON analytics.sessions(project_id,visitor_id,started_at);

CREATE TABLE analytics.session_features (
    session_id uuid PRIMARY KEY REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    duration_ms bigint NOT NULL,
    active_time_ms bigint NOT NULL,
    page_count integer NOT NULL
);
CREATE TABLE analytics.session_pages (
    session_id uuid REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    page_index integer NOT NULL,
    page_url text NOT NULL,
    viewed_at timestamptz NOT NULL,
    PRIMARY KEY(session_id,page_index)
);
CREATE TABLE analytics.business_events (
    id bigint PRIMARY KEY,
    project_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    event_time timestamptz NOT NULL,
    page_url text NOT NULL,
    name text NOT NULL,
    properties jsonb NOT NULL
);
CREATE INDEX analytics_business_session_idx ON analytics.business_events(session_id,event_time,id);
CREATE INDEX analytics_business_report_idx ON analytics.business_events(project_id,name,event_time);
CREATE TABLE analytics.heatmap_events (
    session_id uuid REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    batch_id uuid NOT NULL,
    event_index integer NOT NULL,
    event_time timestamptz NOT NULL,
    page_url text NOT NULL,
    event_type text NOT NULL,
    viewport_width integer NOT NULL,
    viewport_height integer NOT NULL,
    document_height double precision NOT NULL,
    x double precision NOT NULL,
    y double precision NOT NULL,
    depth double precision NOT NULL,
    PRIMARY KEY(session_id,batch_id,event_index)
);
CREATE INDEX analytics_heatmap_page_idx ON analytics.heatmap_events(page_url,viewport_width,event_time);
CREATE TABLE analytics.lead_outcomes (
    id bigint PRIMARY KEY,
    project_id uuid NOT NULL,
    session_id uuid REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    status text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX analytics_outcomes_session_idx ON analytics.lead_outcomes(session_id);
CREATE TABLE analytics.replay_recordings (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    session_id uuid REFERENCES analytics.sessions(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX analytics_replay_session_idx ON analytics.replay_recordings(session_id);

-- Move module-owned configuration; remove the dependency on public.projects.
DO $$
DECLARE t text; c record;
BEGIN
    FOREACH t IN ARRAY ARRAY['analytics_definitions','analytics_api_keys','tracked_links','tracking_pixels','tracking_hits','analytics_annotations'] LOOP
        FOR c IN SELECT conname FROM pg_constraint WHERE conrelid=t::regclass AND confrelid='projects'::regclass LOOP
            EXECUTE format('ALTER TABLE %I DROP CONSTRAINT %I',t,c.conname);
        END LOOP;
        EXECUTE format('ALTER TABLE %I SET SCHEMA analytics',t);
    END LOOP;
END $$;

CREATE TABLE analytics_projection_outbox (
    id bigserial PRIMARY KEY,
    session_id uuid NOT NULL,
    project_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analytics_projection_session_idx ON analytics_projection_outbox(session_id,id);

CREATE FUNCTION enqueue_analytics_projection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; pid uuid;
BEGIN
    IF TG_TABLE_NAME='sessions' THEN
        IF TG_OP='DELETE' THEN sid=OLD.id;pid=OLD.project_id; ELSE sid=NEW.id;pid=NEW.project_id; END IF;
    ELSE
        IF TG_OP='DELETE' THEN sid=OLD.session_id; ELSE sid=NEW.session_id; END IF;
        SELECT project_id INTO pid FROM sessions WHERE id=sid;
    END IF;
    IF pid IS NOT NULL THEN
        INSERT INTO analytics_projection_outbox(session_id,project_id) VALUES(sid,pid);
    END IF;
    RETURN NULL;
END $$;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['sessions','session_features','session_pages','business_events','heatmap_events','lead_outcomes','replay_recordings'] LOOP
        EXECUTE format('CREATE TRIGGER analytics_projection AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION enqueue_analytics_projection()',t);
    END LOOP;
END $$;

INSERT INTO analytics_projection_outbox(session_id,project_id) SELECT id,project_id FROM sessions;
