UPDATE projects
SET scoring_config = jsonb_set(scoring_config, '{weights,AUTOMATION_DETECTED}', '70'::jsonb)
WHERE scoring_config #>> '{weights,AUTOMATION_DETECTED}' = '45';

UPDATE projects
SET scoring_config = jsonb_set(scoring_config, '{limits,high_mouse_velocity}', '6000'::jsonb)
WHERE scoring_config #>> '{limits,high_mouse_velocity}' = '3500';

-- Re-score recent automatic classifications of crawlers and automated
-- browsers under the new rules while their raw events are still retained.
UPDATE sessions
SET finished_at = NULL, finish_requested_at = NULL, updated_at = now()
WHERE manual_label = ''
  AND started_at > now() - interval '6 days'
  AND current_classification IN ('HUMAN', 'SUSPICIOUS', 'BOT')
  AND (automation <> '{}'::jsonb
       OR user_agent ~* '(bot/|crawler|spider|\+https?://|externalagent|externalfetcher|webindexer|googleother|lighthouse|headlesschrome|python-|curl/|wget/|pr-cy|go-http-client)');
