-- source control (源头管控): budgets go through tiered approval (D / CTO by amount)
ALTER TABLE budgets ADD COLUMN approval_status VARCHAR(16) NOT NULL DEFAULT 'approved'; -- pending | approved | rejected
ALTER TABLE budgets ADD COLUMN approver_level  VARCHAR(16) NOT NULL DEFAULT '';         -- D | CTO
ALTER TABLE budgets ADD COLUMN approver        VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE budgets ADD COLUMN applicant       VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE budgets ADD COLUMN project         VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE budgets ADD COLUMN reason          TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets ADD COLUMN reject_reason   TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets ADD COLUMN approved_at     TIMESTAMPTZ;

-- a rejected budget must not block re-applying for the same key
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_key_id_key;
CREATE UNIQUE INDEX uq_budgets_key_open ON budgets (key_id) WHERE approval_status <> 'rejected';
