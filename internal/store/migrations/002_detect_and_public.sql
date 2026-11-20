-- Build auto-detection results and free public URLs (Cloudflare quick tunnels).

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS framework    TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS node_version TEXT NOT NULL DEFAULT '';

ALTER TABLE projects ADD COLUMN IF NOT EXISTS public_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS public_url     TEXT    NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS public_status  TEXT    NOT NULL DEFAULT '';
