-- Re-score recent unengaged ~5 second sessions under ABANDONED_AFTER_FIRST_FLUSH
-- while their raw events are still retained.
UPDATE sessions s
SET finished_at = NULL, finish_requested_at = NULL, updated_at = now()
FROM session_features f
WHERE f.session_id = s.id
  AND s.manual_label = ''
  AND s.current_classification = 'HUMAN'
  AND s.started_at > now() - interval '6 days'
  AND f.duration_ms BETWEEN 4000 AND 6500
  AND f.mouse_moves = 0 AND f.click_count = 0 AND f.scroll_events = 0;
