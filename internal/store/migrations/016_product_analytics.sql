CREATE TABLE business_events (
 id bigserial PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 batch_id uuid NOT NULL,
 event_index integer NOT NULL,
 event_time timestamptz NOT NULL,
 page_url text NOT NULL,
 name varchar(80) NOT NULL,
 properties jsonb NOT NULL DEFAULT '{}',
 UNIQUE(session_id,batch_id,event_index)
);
CREATE INDEX business_events_report_idx ON business_events(project_id,name,event_time);
CREATE INDEX business_events_session_idx ON business_events(session_id,event_time,id);
CREATE TABLE analytics_definitions (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('goal','funnel')),
 name varchar(120) NOT NULL,
 steps jsonb NOT NULL,
 window_minutes integer NOT NULL DEFAULT 60 CHECK(window_minutes BETWEEN 1 AND 10080),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analytics_definitions_project_idx ON analytics_definitions(project_id,kind);
CREATE TABLE heatmap_events (
 session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 batch_id uuid NOT NULL,
 event_index integer NOT NULL,
 event_time timestamptz NOT NULL,
 page_url text NOT NULL,
 event_type text NOT NULL CHECK(event_type IN ('click','scroll')),
 viewport_width integer NOT NULL,
 viewport_height integer NOT NULL,
 document_height double precision NOT NULL,
 x double precision NOT NULL,
 y double precision NOT NULL,
 depth double precision NOT NULL,
 PRIMARY KEY(session_id,batch_id,event_index)
);
CREATE INDEX heatmap_page_idx ON heatmap_events(page_url,viewport_width,event_time);
ALTER TABLE sessions ADD COLUMN browser text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN os text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN region text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN city text NOT NULL DEFAULT '';
