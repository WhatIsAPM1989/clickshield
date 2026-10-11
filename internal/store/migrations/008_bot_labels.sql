-- Which known bot or automation client a session came from. NULL means the
-- session has not been labeled yet; the server labels such sessions on start,
-- so a catalog change only needs to reset bot_name to NULL.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS bot_name text;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS bot_category text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS sessions_project_bot_idx ON sessions(project_id, bot_name) WHERE bot_name <> '';
CREATE INDEX IF NOT EXISTS sessions_unlabeled_bot_idx ON sessions(id) WHERE bot_name IS NULL;
