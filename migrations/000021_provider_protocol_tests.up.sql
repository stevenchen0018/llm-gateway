-- 供应商协议：openai（含所有 OpenAI 兼容接口）| anthropic（Messages API）| gemini（generateContent）
ALTER TABLE providers ADD COLUMN protocol VARCHAR(16) NOT NULL DEFAULT 'openai';
UPDATE providers SET protocol = 'anthropic' WHERE base_url ILIKE '%anthropic.com%';
UPDATE providers SET protocol = 'gemini' WHERE base_url ILIKE '%generativelanguage.googleapis.com%';

-- 供应商可用性测试：一次测试（手动 / 批量 / 定时巡检）
CREATE TABLE provider_test_runs (
    id              BIGSERIAL PRIMARY KEY,
    provider_id     BIGINT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    trigger         VARCHAR(16) NOT NULL DEFAULT 'manual',   -- manual | batch | scheduled
    operator        VARCHAR(64) NOT NULL DEFAULT '',
    protocol        VARCHAR(16) NOT NULL DEFAULT 'openai',   -- 测试时供应商使用的协议
    status          VARCHAR(16) NOT NULL DEFAULT 'running',  -- running | success | partial | failed
    total           INTEGER NOT NULL DEFAULT 0,
    passed          INTEGER NOT NULL DEFAULT 0,
    failed          INTEGER NOT NULL DEFAULT 0,
    skipped         INTEGER NOT NULL DEFAULT 0,
    avg_latency_ms  INTEGER NOT NULL DEFAULT 0,
    p95_latency_ms  INTEGER NOT NULL DEFAULT 0,
    summary         TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ
);
CREATE INDEX idx_provider_test_runs_provider ON provider_test_runs (provider_id, started_at DESC);
CREATE INDEX idx_provider_test_runs_started ON provider_test_runs (started_at);

-- 测试明细：连通性探测 + 每个模型每轮一次真实调用
CREATE TABLE provider_test_results (
    id                 BIGSERIAL PRIMARY KEY,
    run_id             BIGINT NOT NULL REFERENCES provider_test_runs(id) ON DELETE CASCADE,
    provider_id        BIGINT NOT NULL,
    model_id           BIGINT,
    model_key          VARCHAR(128) NOT NULL DEFAULT '',
    check_type         VARCHAR(16) NOT NULL,                 -- probe | chat | embedding
    round              INTEGER NOT NULL DEFAULT 1,
    status             VARCHAR(16) NOT NULL,                 -- passed | failed | skipped
    latency_ms         INTEGER NOT NULL DEFAULT 0,
    http_status        INTEGER NOT NULL DEFAULT 0,
    prompt_tokens      INTEGER NOT NULL DEFAULT 0,
    completion_tokens  INTEGER NOT NULL DEFAULT 0,
    error_type         VARCHAR(32) NOT NULL DEFAULT '',      -- timeout | network | auth | not_found | rate_limited | upstream_5xx | bad_request | invalid_response | unsupported
    error_message      TEXT NOT NULL DEFAULT '',
    response_preview   TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_provider_test_results_run ON provider_test_results (run_id);
CREATE INDEX idx_provider_test_results_provider ON provider_test_results (provider_id, created_at);
