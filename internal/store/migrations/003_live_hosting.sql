-- Permanent live hosting (Netlify): encrypted platform settings and the
-- live site / deploy ids that make publishing and instant rollback work.

CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    ciphertext  BYTEA NOT NULL,
    nonce       BYTEA NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE projects    ADD COLUMN IF NOT EXISTS live_site_id   TEXT NOT NULL DEFAULT '';
ALTER TABLE projects    ADD COLUMN IF NOT EXISTS live_url       TEXT NOT NULL DEFAULT '';
ALTER TABLE projects    ADD COLUMN IF NOT EXISTS live_status    TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS live_deploy_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS live_url       TEXT NOT NULL DEFAULT '';
