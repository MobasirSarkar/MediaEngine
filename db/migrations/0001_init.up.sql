CREATE TABLE IF NOT EXISTS uploads (
    id              UUID PRIMARY KEY,
    owner_id        TEXT NOT NULL,
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    total_size      BIGINT NOT NULL,
    chunk_size      INT NOT NULL,
    total_chunks    INT NOT NULL,
    received_bytes  BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,
    bucket          TEXT NOT NULL,
    key             TEXT NOT NULL,
    checksum_sha256 TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_uploads_owner ON uploads(owner_id);
CREATE INDEX IF NOT EXISTS idx_uploads_status ON uploads(status);

CREATE TABLE IF NOT EXISTS chunks (
    upload_id   UUID NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
    chunk_no    INT  NOT NULL,
    size        INT  NOT NULL,
    checksum    TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (upload_id, chunk_no)
);

CREATE TABLE IF NOT EXISTS jobs (
    id           UUID PRIMARY KEY,
    upload_id    UUID NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
    status       TEXT NOT NULL,
    error_code   TEXT,
    error_msg    TEXT,
    retries      INT  NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_jobs_upload ON jobs(upload_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);

CREATE TABLE IF NOT EXISTS tasks (
    id          UUID PRIMARY KEY,
    job_id      UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    status      TEXT NOT NULL,
    attempt     INT  NOT NULL DEFAULT 0,
    payload     JSONB,
    result      JSONB,
    error_msg   TEXT,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tasks_job ON tasks(job_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
