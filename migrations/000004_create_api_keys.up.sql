CREATE TABLE api_keys (
    id            BIGSERIAL PRIMARY KEY,
    key_prefix    VARCHAR(16) NOT NULL UNIQUE,
    key_hash      VARCHAR(64) NOT NULL UNIQUE,
    name          VARCHAR(128) NOT NULL,
    scenario      VARCHAR(256) NOT NULL DEFAULT '',
    owner         VARCHAR(128) NOT NULL,
    shared_users  TEXT NOT NULL DEFAULT '[]',
    status        VARCHAR(16) NOT NULL DEFAULT 'pending',
    tpm_quota     INTEGER NOT NULL DEFAULT 0,
    qps_quota     INTEGER NOT NULL DEFAULT 0,
    budget_id     BIGINT,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_status ON api_keys (status);
