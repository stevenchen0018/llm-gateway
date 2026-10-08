CREATE TABLE models (
    id                   BIGSERIAL PRIMARY KEY,
    provider_id          BIGINT NOT NULL REFERENCES providers (id) ON DELETE CASCADE,
    model_key            VARCHAR(128) NOT NULL,
    display_name         VARCHAR(128) NOT NULL,
    type                 VARCHAR(32) NOT NULL DEFAULT 'chat',
    input_price_per_1k   NUMERIC(18, 8) NOT NULL DEFAULT 0,
    output_price_per_1k  NUMERIC(18, 8) NOT NULL DEFAULT 0,
    tpm_limit            INTEGER NOT NULL DEFAULT 0,
    qps_limit            INTEGER NOT NULL DEFAULT 0,
    status               VARCHAR(16) NOT NULL DEFAULT 'active',
    created_by           VARCHAR(128) NOT NULL DEFAULT '',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_models_provider_id ON models (provider_id);
CREATE INDEX idx_models_status ON models (status);
