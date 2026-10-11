DROP TRIGGER analytics_projection ON sessions;
CREATE TRIGGER analytics_projection AFTER INSERT OR DELETE OR UPDATE OF
    last_seen_at,started_at,traffic_source,traffic_channel,referrer_host,landing_url,
    last_page_url,device_type,device,user_agent,country,region,city,asn,as_org,browser,
    os,page_count,current_classification,current_risk_score,bot_name,bot_category,
    utm_source,utm_medium,utm_campaign,utm_content,utm_term ON sessions
    FOR EACH ROW EXECUTE FUNCTION enqueue_analytics_projection();

CREATE FUNCTION enqueue_analytics_batch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO analytics_projection_outbox(session_id,project_id)
    SELECT DISTINCT c.session_id,s.project_id FROM changed_rows c JOIN sessions s ON s.id=c.session_id;
    RETURN NULL;
END $$;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['session_features','session_pages','business_events','heatmap_events','lead_outcomes','replay_recordings'] LOOP
        EXECUTE format('DROP TRIGGER analytics_projection ON %I',t);
        EXECUTE format('CREATE TRIGGER analytics_insert AFTER INSERT ON %I REFERENCING NEW TABLE AS changed_rows FOR EACH STATEMENT EXECUTE FUNCTION enqueue_analytics_batch()',t);
        EXECUTE format('CREATE TRIGGER analytics_update AFTER UPDATE ON %I REFERENCING NEW TABLE AS changed_rows FOR EACH STATEMENT EXECUTE FUNCTION enqueue_analytics_batch()',t);
        EXECUTE format('CREATE TRIGGER analytics_delete AFTER DELETE ON %I REFERENCING OLD TABLE AS changed_rows FOR EACH STATEMENT EXECUTE FUNCTION enqueue_analytics_batch()',t);
    END LOOP;
END $$;
