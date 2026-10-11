-- NOTIFY sends classifications to integrations without blocking, and
-- BLOCKING_BOTS blocks only BOT visitors. MONITORING no longer sends
-- anything, so projects that relied on it for deliveries move to NOTIFY.
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_mode_check;
ALTER TABLE projects ADD CONSTRAINT projects_mode_check CHECK (mode IN ('MONITORING','NOTIFY','BLOCKING','BLOCKING_BOTS'));
UPDATE projects SET mode='NOTIFY',updated_at=now() WHERE mode='MONITORING' AND EXISTS(SELECT 1 FROM integrations i WHERE i.project_id=projects.id AND i.enabled);
