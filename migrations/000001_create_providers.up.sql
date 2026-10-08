CREATE TABLE providers (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(64) NOT NULL UNIQUE,
    name        VARCHAR(128) NOT NULL,
    base_url    VARCHAR(512) NOT NULL,
    auth_type   VARCHAR(32) NOT NULL DEFAULT '',
    auth_value  VARCHAR(512) NOT NULL DEFAULT '',
    status      VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_providers_status ON providers (status);
