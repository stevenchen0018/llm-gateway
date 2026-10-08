CREATE TABLE audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    key_id        BIGINT NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    action        VARCHAR(32) NOT NULL,
    operator      VARCHAR(128) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_key_id ON audit_logs (key_id);
