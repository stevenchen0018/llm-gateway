DROP INDEX IF EXISTS idx_alert_events_key_id;
ALTER TABLE alert_events DROP COLUMN IF EXISTS key_id;

DROP INDEX IF EXISTS uq_my_models_user_model;
DELETE FROM my_models a USING my_models b WHERE a.model_id = b.model_id AND a.id > b.id;
ALTER TABLE my_models DROP COLUMN IF EXISTS user_id;
ALTER TABLE my_models ADD CONSTRAINT my_models_model_id_key UNIQUE (model_id);

ALTER TABLE applications ADD COLUMN department VARCHAR(128) NOT NULL DEFAULT '';
UPDATE applications a SET department = d.name FROM departments d WHERE d.id = a.department_id;
DROP INDEX IF EXISTS idx_applications_department;
ALTER TABLE applications DROP COLUMN department_id;

DROP TABLE IF EXISTS admin_users;
DROP TABLE IF EXISTS departments;
