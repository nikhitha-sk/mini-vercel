-- Mini Vercel — initial schema.
-- Deployments are immutable once READY; the aliases table is the routing
-- table, so promote / rollback is a single UPDATE.

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    github_id   BIGINT UNIQUE,
    login       TEXT NOT NULL,
    email       TEXT,
    avatar_url  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id              TEXT PRIMARY KEY,
    owner_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    repo_url        TEXT NOT NULL DEFAULT '',
    branch          TEXT NOT NULL DEFAULT 'main',
    framework       TEXT NOT NULL DEFAULT 'vite',
    install_cmd     TEXT NOT NULL DEFAULT '',
    build_cmd       TEXT NOT NULL DEFAULT '',
    output_dir      TEXT NOT NULL DEFAULT 'dist',
    root_dir        TEXT NOT NULL DEFAULT '',
    webhook_secret  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS deployments (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source          TEXT NOT NULL,              -- 'git' | 'zip'
    trigger         TEXT NOT NULL DEFAULT 'manual', -- 'manual' | 'api' | 'webhook' | 'redeploy'
    branch          TEXT NOT NULL DEFAULT '',
    commit_sha      TEXT NOT NULL DEFAULT '',
    commit_message  TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'QUEUED',
    error           TEXT NOT NULL DEFAULT '',
    attempt         INT  NOT NULL DEFAULT 0,
    file_count      INT  NOT NULL DEFAULT 0,
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    -- snapshot of the build config at deploy time (immutable)
    install_cmd     TEXT NOT NULL DEFAULT '',
    build_cmd       TEXT NOT NULL DEFAULT '',
    output_dir      TEXT NOT NULL DEFAULT '',
    root_dir        TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    CONSTRAINT deployments_status_chk CHECK (status IN ('QUEUED','BUILDING','UPLOADING','READY','FAILED','CANCELED'))
);
CREATE INDEX IF NOT EXISTS deployments_project_created_idx ON deployments (project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS aliases (
    hostname       TEXT PRIMARY KEY,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    deployment_id  TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS aliases_project_idx ON aliases (project_id);

CREATE TABLE IF NOT EXISTS env_vars (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    ciphertext  BYTEA NOT NULL,
    nonce       BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, key)
);

CREATE TABLE IF NOT EXISTS build_logs (
    deployment_id  TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    seq            INT  NOT NULL,
    ts             TIMESTAMPTZ NOT NULL DEFAULT now(),
    stream         TEXT NOT NULL,  -- 'stdout' | 'stderr' | 'system'
    line           TEXT NOT NULL,
    PRIMARY KEY (deployment_id, seq)
);
