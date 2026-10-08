DROP INDEX IF EXISTS uq_budgets_key_open;
ALTER TABLE budgets ADD CONSTRAINT budgets_key_id_key UNIQUE (key_id);
ALTER TABLE budgets DROP COLUMN approved_at, DROP COLUMN reject_reason, DROP COLUMN reason, DROP COLUMN project, DROP COLUMN applicant, DROP COLUMN approver, DROP COLUMN approver_level, DROP COLUMN approval_status;
