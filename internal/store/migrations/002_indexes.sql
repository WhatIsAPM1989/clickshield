CREATE INDEX IF NOT EXISTS sessions_project_visitor_started_idx
    ON sessions(project_id, visitor_id, started_at DESC);

CREATE INDEX IF NOT EXISTS sessions_paid_started_idx
    ON sessions(project_id, started_at DESC)
    WHERE yclid <> '' OR gclid <> '' OR lower(utm_medium) IN ('cpc','ppc','paid','paidsearch','display');
