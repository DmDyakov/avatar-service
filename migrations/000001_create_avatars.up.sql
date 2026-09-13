CREATE TABLE IF NOT EXISTS avatars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id VARCHAR(255) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    s3_key VARCHAR(500) NOT NULL UNIQUE,
    upload_status VARCHAR(50) NOT NULL DEFAULT 'uploading',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT chk_upload_status CHECK (
        upload_status IN ('uploading', 'uploaded', 'failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_avatars_user_id
    ON avatars(user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_avatars_status
    ON avatars(upload_status);

CREATE INDEX IF NOT EXISTS idx_avatars_created_at
    ON avatars(created_at DESC);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_avatars_updated_at
    BEFORE UPDATE ON avatars
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
