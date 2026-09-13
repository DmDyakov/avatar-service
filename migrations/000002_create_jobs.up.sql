CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    avatar_id UUID NOT NULL REFERENCES avatars(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    CONSTRAINT chk_jobs_type CHECK (
        type IN ('generate_thumbnails', 'delete_files')
    ),
    CONSTRAINT chk_jobs_status CHECK (
        status IN ('pending', 'processing', 'completed', 'failed')
    ),
    CONSTRAINT chk_jobs_attempts CHECK (attempts >= 0),
    CONSTRAINT chk_jobs_max_attempts CHECK (max_attempts > 0)
);

CREATE INDEX IF NOT EXISTS idx_jobs_avatar_id
    ON jobs(avatar_id);

CREATE INDEX IF NOT EXISTS idx_jobs_status
    ON jobs(status);

CREATE INDEX IF NOT EXISTS idx_jobs_type_status
    ON jobs(type, status);

CREATE INDEX IF NOT EXISTS idx_jobs_created_at
    ON jobs(created_at DESC);

CREATE TRIGGER trg_jobs_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
