-- Key IP whitelist: JSON array of IPs / CIDRs; empty = any source allowed
ALTER TABLE api_keys ADD COLUMN ip_whitelist TEXT NOT NULL DEFAULT '[]';

-- Runtime switches editable from the console without a restart
CREATE TABLE gateway_settings (
    key         VARCHAR(64) PRIMARY KEY,
    value       JSONB NOT NULL,
    updated_by  VARCHAR(64) NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Prompt / content filter rules. department_id NULL = platform-wide rule.
CREATE TABLE content_filter_rules (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(128) NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    match_type     VARCHAR(16) NOT NULL CHECK (match_type IN ('keyword', 'regex')),
    pattern        TEXT NOT NULL,
    action         VARCHAR(16) NOT NULL CHECK (action IN ('block', 'mask', 'log')),
    replacement    VARCHAR(64) NOT NULL DEFAULT '***',
    stage          VARCHAR(16) NOT NULL DEFAULT 'input' CHECK (stage IN ('input', 'output', 'both')),
    department_id  BIGINT REFERENCES departments (id) ON DELETE CASCADE,
    priority       INTEGER NOT NULL DEFAULT 100,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    hit_count      BIGINT NOT NULL DEFAULT 0,
    last_hit_at    TIMESTAMPTZ,
    created_by     VARCHAR(64) NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_content_filter_rules_dept ON content_filter_rules (department_id);

-- Full request/response records (prompt audit), written asynchronously
CREATE TABLE request_logs (
    id                 BIGSERIAL PRIMARY KEY,
    request_id         VARCHAR(64) NOT NULL,
    key_id             BIGINT NOT NULL,
    endpoint           VARCHAR(64) NOT NULL,
    model              VARCHAR(128) NOT NULL DEFAULT '',
    model_id           BIGINT,
    provider_id        BIGINT,
    status             VARCHAR(16) NOT NULL,          -- success | failed | blocked
    http_status        INTEGER NOT NULL,
    error_code         VARCHAR(64) NOT NULL DEFAULT '',
    prompt_preview     TEXT NOT NULL DEFAULT '',
    request_body       TEXT NOT NULL DEFAULT '',
    response_body      TEXT NOT NULL DEFAULT '',
    body_truncated     BOOLEAN NOT NULL DEFAULT FALSE,
    filter_hits        JSONB NOT NULL DEFAULT '[]',
    prompt_tokens      INTEGER NOT NULL DEFAULT 0,
    completion_tokens  INTEGER NOT NULL DEFAULT 0,
    latency_ms         INTEGER NOT NULL DEFAULT 0,
    source_ip          VARCHAR(64) NOT NULL DEFAULT '',
    user_agent         VARCHAR(256) NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_request_logs_created_at ON request_logs (created_at);
CREATE INDEX idx_request_logs_key_created ON request_logs (key_id, created_at);
CREATE INDEX idx_request_logs_request_id ON request_logs (request_id);
