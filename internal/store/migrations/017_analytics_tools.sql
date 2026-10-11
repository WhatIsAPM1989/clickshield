CREATE TABLE analytics_api_keys (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 name varchar(120) NOT NULL,
 token_hash bytea NOT NULL UNIQUE,
 prefix text NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 last_used_at timestamptz
);
CREATE INDEX analytics_api_keys_project_idx ON analytics_api_keys(project_id);
CREATE TABLE tracked_links (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 slug text NOT NULL UNIQUE,
 name varchar(120) NOT NULL,
 destination text NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE tracking_pixels (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 name varchar(120) NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE tracking_hits (
 id bigserial PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 link_id uuid REFERENCES tracked_links(id) ON DELETE CASCADE,
 pixel_id uuid REFERENCES tracking_pixels(id) ON DELETE CASCADE,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 country text NOT NULL DEFAULT '',
 browser text NOT NULL DEFAULT '',
 os text NOT NULL DEFAULT '',
 referrer text NOT NULL DEFAULT '',
 CHECK((link_id IS NULL) <> (pixel_id IS NULL))
);
CREATE INDEX tracking_hits_project_idx ON tracking_hits(project_id,occurred_at);
CREATE TABLE analytics_annotations (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 at timestamptz NOT NULL,
 text varchar(500) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analytics_annotations_project_idx ON analytics_annotations(project_id,at);
