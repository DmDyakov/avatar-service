CREATE TABLE IF NOT EXISTS avatars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id VARCHAR(255) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    s3_key VARCHAR(500) NOT NULL UNIQUE,

    -- Размеры оригинала. Заполняются при upload (imaging.Decode).
    -- NULL = ещё не декодировали (или старая запись).
    width INT,
    height INT,

    -- Ключи миниатюр: {"100x100": "thumbnails/{id}/100x100.jpg", "300x300": "..."}.
    -- Заполняются воркером (Sprint 2).
    thumbnail_s3_keys JSONB,

    upload_status VARCHAR(50) NOT NULL DEFAULT 'uploading',
    processing_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT chk_upload_status CHECK (
        upload_status IN ('uploading', 'uploaded', 'failed')
    ),
    CONSTRAINT chk_processing_status CHECK (
        processing_status IN ('pending', 'processing', 'completed', 'failed')
    ),
    CONSTRAINT chk_avatars_attempts CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_avatars_user_id
    ON avatars(user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_avatars_status
    ON avatars(upload_status, processing_status);

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
