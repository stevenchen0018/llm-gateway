CREATE TABLE model_routes (
    id                  BIGSERIAL PRIMARY KEY,
    alias               VARCHAR(128) NOT NULL,
    candidate_model_id  BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    priority            INTEGER NOT NULL DEFAULT 0,
    weight              INTEGER NOT NULL DEFAULT 1,
    strategy            VARCHAR(32) NOT NULL DEFAULT 'priority',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_model_routes_alias ON model_routes (alias);
