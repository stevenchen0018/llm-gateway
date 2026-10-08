CREATE TABLE alert_events (
    id            BIGSERIAL PRIMARY KEY,
    type          VARCHAR(16) NOT NULL,
    ref_id        BIGINT NOT NULL DEFAULT 0,
    message       TEXT NOT NULL DEFAULT '',
    level         VARCHAR(16) NOT NULL DEFAULT 'info',
    notified_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_alert_events_type ON alert_events (type);
CREATE INDEX idx_alert_events_created_at ON alert_events (created_at);
