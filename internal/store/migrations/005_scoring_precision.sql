ALTER TABLE session_features
    ADD COLUMN IF NOT EXISTS mouse_p95_velocity double precision NOT NULL DEFAULT 0;

UPDATE projects
SET scoring_config = jsonb_set(scoring_config, '{weights,RAPID_UNENGAGED_PAGE_BURST}', '75'::jsonb)
WHERE scoring_config #>> '{weights,RAPID_UNENGAGED_PAGE_BURST}' = '45';
