CREATE TABLE ingest_batches (
    session_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    visitor_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id,batch_id)
);
CREATE INDEX ingest_batches_created_idx ON ingest_batches(created_at);
INSERT INTO ingest_batches(session_id,batch_id,project_id,visitor_key,created_at)
SELECT r.session_id,r.batch_id,r.project_id,s.visitor_key,min(r.created_at)
FROM raw_events r JOIN sessions s ON s.id=r.session_id
GROUP BY r.session_id,r.batch_id,r.project_id,s.visitor_key;
