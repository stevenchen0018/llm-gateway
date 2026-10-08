CREATE TABLE my_models (
    id          BIGSERIAL PRIMARY KEY,
    model_id    BIGINT NOT NULL UNIQUE REFERENCES models (id) ON DELETE CASCADE,
    created_by  VARCHAR(128) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE announcements (
    id          BIGSERIAL PRIMARY KEY,
    content     TEXT NOT NULL,
    level       VARCHAR(16) NOT NULL DEFAULT 'info',
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
