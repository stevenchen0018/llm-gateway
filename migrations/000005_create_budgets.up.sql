CREATE TABLE budgets (
    id                    BIGSERIAL PRIMARY KEY,
    key_id                BIGINT NOT NULL UNIQUE REFERENCES api_keys (id) ON DELETE CASCADE,
    period                VARCHAR(16) NOT NULL DEFAULT 'monthly',
    amount                NUMERIC(18, 4) NOT NULL DEFAULT 0,
    consumed              NUMERIC(18, 4) NOT NULL DEFAULT 0,
    currency              VARCHAR(8) NOT NULL DEFAULT 'CNY',
    alert_threshold_pct   INTEGER NOT NULL DEFAULT 80,
    status                VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE api_keys ADD CONSTRAINT fk_api_keys_budget FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE SET NULL;

CREATE INDEX idx_budgets_status ON budgets (status);
