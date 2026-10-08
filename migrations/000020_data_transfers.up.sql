-- record of bulk imports (committed) and exports, for audit and the 导入导出 hub
CREATE TABLE data_transfers (
    id             BIGSERIAL PRIMARY KEY,
    direction      VARCHAR(8) NOT NULL,           -- import | export
    entity         VARCHAR(32) NOT NULL,
    entity_title   VARCHAR(64) NOT NULL DEFAULT '',
    format         VARCHAR(8) NOT NULL DEFAULT 'xlsx',
    file_name      VARCHAR(256) NOT NULL DEFAULT '',
    operator       VARCHAR(64) NOT NULL DEFAULT '',
    operator_id    BIGINT,
    department_id  BIGINT,
    status         VARCHAR(16) NOT NULL,          -- success | partial | failed
    total          INTEGER NOT NULL DEFAULT 0,
    created        INTEGER NOT NULL DEFAULT 0,
    updated        INTEGER NOT NULL DEFAULT 0,
    skipped        INTEGER NOT NULL DEFAULT 0,
    failed         INTEGER NOT NULL DEFAULT 0,
    message        TEXT NOT NULL DEFAULT '',
    errors         JSONB NOT NULL DEFAULT '[]',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_data_transfers_created ON data_transfers (created_at);
CREATE INDEX idx_data_transfers_dept ON data_transfers (department_id);
