UPDATE projects
SET scoring_config = jsonb_set(
    scoring_config,
    '{weights}',
    COALESCE(scoring_config->'weights', '{}'::jsonb) || '{"RAPID_UNENGAGED_PAGE_BURST":45}'::jsonb
)
WHERE NOT COALESCE(scoring_config->'weights', '{}'::jsonb) ? 'RAPID_UNENGAGED_PAGE_BURST';
