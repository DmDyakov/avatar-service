CREATE TYPE outbox_status AS ENUM ('pending', 'in_progress', 'published', 'failed');

CREATE TABLE outbox_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type     VARCHAR(100) NOT NULL,
    payload        JSONB NOT NULL,
    status         outbox_status NOT NULL DEFAULT 'pending',
    attempts       INT NOT NULL DEFAULT 0,
    locked_at      TIMESTAMPTZ,
    locked_by      VARCHAR(128),
    failure_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending
    ON outbox_events (created_at)
    WHERE status IN ('pending', 'failed');

CREATE INDEX idx_outbox_locked
    ON outbox_events (locked_at)
    WHERE status = 'in_progress';
