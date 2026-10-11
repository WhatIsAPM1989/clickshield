ALTER TABLE replay_chunks RENAME TO replay_chunks_legacy;
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
    object_key text NOT NULL UNIQUE,
    checksum text NOT NULL,
    raw_bytes integer NOT NULL,
    ready boolean NOT NULL DEFAULT false,
    PRIMARY KEY(recording_id,page_id,sequence,part_index)
);
CREATE TABLE replay_object_gc (
    object_key text PRIMARY KEY,
    not_before timestamptz NOT NULL DEFAULT now()+interval '5 minutes',
    attempts integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT ''
);
CREATE FUNCTION enqueue_replay_object_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO replay_object_gc(object_key) VALUES(OLD.object_key) ON CONFLICT DO NOTHING;
    RETURN NULL;
END $$;
CREATE TRIGGER replay_object_delete AFTER DELETE ON replay_chunks
    FOR EACH ROW EXECUTE FUNCTION enqueue_replay_object_delete();

-- Old payloads remain recoverable until explicit `backfill replay` finishes.
-- Fresh installations have no legacy payload table at all.
DO $$ BEGIN
    IF NOT EXISTS(SELECT 1 FROM replay_chunks_legacy) THEN DROP TABLE replay_chunks_legacy; END IF;
END $$;
