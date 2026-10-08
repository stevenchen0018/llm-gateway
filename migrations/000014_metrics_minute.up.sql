-- Minute-level rollup of gateway traffic: the in-process equivalent of the
-- article's Flink real-time aggregation. Every monitoring/cost dashboard
-- reads this table; usage_records stays the raw per-call log.
CREATE TABLE metrics_minute (
    bucket             TIMESTAMPTZ NOT NULL,
    key_id             BIGINT NOT NULL,
    model_id           BIGINT NOT NULL,
    provider_id        BIGINT NOT NULL,
    requests           BIGINT NOT NULL DEFAULT 0,
    failed             BIGINT NOT NULL DEFAULT 0,
    prompt_tokens      BIGINT NOT NULL DEFAULT 0,
    completion_tokens  BIGINT NOT NULL DEFAULT 0,
    total_tokens       BIGINT NOT NULL DEFAULT 0,
    latency_sum_ms     BIGINT NOT NULL DEFAULT 0,
    cost               NUMERIC(18, 8) NOT NULL DEFAULT 0,
    list_cost          NUMERIC(18, 8) NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket, key_id, model_id)
);
CREATE INDEX idx_metrics_minute_model ON metrics_minute (model_id, bucket);
CREATE INDEX idx_metrics_minute_key ON metrics_minute (key_id, bucket);
CREATE INDEX idx_metrics_minute_provider ON metrics_minute (provider_id, bucket);

ALTER TABLE usage_records ADD COLUMN list_cost NUMERIC(18, 8) NOT NULL DEFAULT 0;
UPDATE usage_records SET list_cost = cost WHERE list_cost = 0;
CREATE INDEX idx_usage_records_created_at ON usage_records (created_at);
