CREATE TABLE applications (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(128) NOT NULL UNIQUE,
    description    TEXT NOT NULL DEFAULT '',
    department     VARCHAR(128) NOT NULL DEFAULT '',
    owner          VARCHAR(128) NOT NULL DEFAULT '',
    owner_email    VARCHAR(256) NOT NULL DEFAULT '',
    manager        VARCHAR(128) NOT NULL DEFAULT '',
    manager_email  VARCHAR(256) NOT NULL DEFAULT '',
    status         VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
