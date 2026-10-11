ALTER TABLE sessions ADD COLUMN data_revision bigint NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN scoring_lease uuid;
ALTER TABLE sessions ADD COLUMN scoring_lease_until timestamptz;
ALTER TABLE sessions ADD COLUMN scored_revision bigint;

ALTER TABLE classifications ADD COLUMN algorithm_version text NOT NULL DEFAULT 'legacy';
ALTER TABLE classifications ADD COLUMN input_revision bigint;
ALTER TABLE classifications ADD COLUMN config_snapshot jsonb;
ALTER TABLE classifications ADD COLUMN context_snapshot jsonb;
ALTER TABLE classifications ADD COLUMN features_snapshot jsonb;
CREATE INDEX sessions_score_lease_idx ON sessions(scoring_lease_until)
    WHERE scoring_lease IS NOT NULL;

-- Older batches could merge JSON null into an array instead of an object.
UPDATE sessions SET automation='{}'::jsonb WHERE jsonb_typeof(automation)<>'object';
