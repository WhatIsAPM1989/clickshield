ALTER TABLE sessions ADD COLUMN score_next_attempt_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE analytics_projection_outbox ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE analytics_projection_outbox ADD COLUMN attempts integer NOT NULL DEFAULT 0;
ALTER TABLE analytics_projection_outbox ADD COLUMN last_error text NOT NULL DEFAULT '';
CREATE INDEX analytics_projection_due_idx ON analytics_projection_outbox(next_attempt_at,id);

-- Projects without sessions still need a durable deletion notification.
CREATE FUNCTION enqueue_analytics_project_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO analytics_projection_outbox(session_id,project_id) VALUES(OLD.id,OLD.id);
    RETURN NULL;
END $$;
CREATE TRIGGER analytics_project_delete AFTER DELETE ON projects
    FOR EACH ROW EXECUTE FUNCTION enqueue_analytics_project_delete();
