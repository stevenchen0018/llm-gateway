CREATE TABLE usage_records (
    id                  BIGSERIAL PRIMARY KEY,
    request_id          VARCHAR(64) NOT NULL,
    key_id               BIGINT NOT NULL,
    model_id             BIGINT NOT NULL,
    provider_id          BIGINT NOT NULL,
    alias                VARCHAR(128) NOT NULL DEFAULT '',
    prompt_tokens        INTEGER NOT NULL DEFAULT 0,
    completion_tokens    INTEGER NOT NULL DEFAULT 0,
    total_tokens         INTEGER NOT NULL DEFAULT 0,
    cost                 NUMERIC(18, 8) NOT NULL DEFAULT 0,
    latency_ms           INTEGER NOT NULL DEFAULT 0,
    status               VARCHAR(16) NOT NULL DEFAULT 'success',
    error_code           VARCHAR(64) NOT NULL DEFAULT '',
    source_ip            VARCHAR(64) NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_usage_records_request_id ON usage_records (request_id);
CREATE INDEX idx_usage_key_created ON usage_records (key_id, created_at);
CREATE INDEX idx_usage_model_created ON usage_records (model_id, created_at);
CREATE INDEX idx_usage_records_provider_id ON usage_records (provider_id);
