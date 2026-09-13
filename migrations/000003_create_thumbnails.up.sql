CREATE TABLE IF NOT EXISTS thumbnails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    avatar_id UUID NOT NULL REFERENCES avatars(id) ON DELETE CASCADE,
    size VARCHAR(50) NOT NULL,
    width INT NOT NULL,
    height INT NOT NULL,
    s3_key VARCHAR(500) NOT NULL UNIQUE,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_thumbnails_avatar_size UNIQUE (avatar_id, size),
    CONSTRAINT chk_thumbnails_size CHECK (size IN ('100x100', '300x300')),
    CONSTRAINT chk_thumbnails_width CHECK (width > 0),
    CONSTRAINT chk_thumbnails_height CHECK (height > 0),
    CONSTRAINT chk_thumbnails_size_bytes CHECK (size_bytes > 0)
);

CREATE INDEX IF NOT EXISTS idx_thumbnails_avatar_id
    ON thumbnails(avatar_id);
