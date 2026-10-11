ALTER TABLE sessions ADD COLUMN traffic_source text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN traffic_channel text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN referrer_host text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN attribution_basis text NOT NULL DEFAULT '';
CREATE INDEX sessions_traffic_idx ON sessions(project_id,traffic_source,traffic_channel,started_at DESC);
ALTER TABLE projects ADD COLUMN replay_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE projects ADD COLUMN replay_sample_rate integer NOT NULL DEFAULT 100 CHECK(replay_sample_rate BETWEEN 0 AND 100);
ALTER TABLE projects ADD COLUMN replay_retention_days integer NOT NULL DEFAULT 7 CHECK(replay_retention_days BETWEEN 1 AND 30);
CREATE TABLE session_pages (
 session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 page_index integer NOT NULL,
 page_url text NOT NULL,
 viewed_at timestamptz NOT NULL,
 PRIMARY KEY(session_id,page_index)
);
CREATE TABLE replay_recordings (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 visitor_key text NOT NULL,
 started_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL,
 total_bytes bigint NOT NULL DEFAULT 0,
 event_count bigint NOT NULL DEFAULT 0
);
CREATE INDEX replay_recordings_session_idx ON replay_recordings(session_id);
CREATE INDEX replay_recordings_expires_idx ON replay_recordings(expires_at);
CREATE TABLE replay_chunks (
 recording_id uuid NOT NULL REFERENCES replay_recordings(id) ON DELETE CASCADE,
 page_id uuid NOT NULL,
 sequence integer NOT NULL,
 part_index integer NOT NULL,
 part_count integer NOT NULL,
 page_url text NOT NULL,
 page_index integer NOT NULL,
 started_at bigint NOT NULL,
 ended_at bigint NOT NULL,
 payload bytea NOT NULL,
 event_count integer NOT NULL,
 PRIMARY KEY(recording_id,page_id,sequence,part_index)
);
