-- Multi-tenancy: departments own applications (and through them API keys,
-- budgets, key-scoped scheduling policies and traffic); console users are
-- scoped to a department unless they are super admins.
CREATE TABLE departments (
    id           BIGSERIAL PRIMARY KEY,
    code         VARCHAR(64)  NOT NULL UNIQUE,
    name         VARCHAR(128) NOT NULL UNIQUE,
    description  TEXT         NOT NULL DEFAULT '',
    leader       VARCHAR(128) NOT NULL DEFAULT '',
    tpm_quota    INTEGER      NOT NULL DEFAULT 0, -- department-wide ceiling, 0 = unlimited
    qps_quota    INTEGER      NOT NULL DEFAULT 0,
    status       VARCHAR(16)  NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE admin_users (
    id             BIGSERIAL PRIMARY KEY,
    username       VARCHAR(64)  NOT NULL UNIQUE,
    password_hash  VARCHAR(100) NOT NULL,
    display_name   VARCHAR(128) NOT NULL DEFAULT '',
    email          VARCHAR(256) NOT NULL DEFAULT '',
    role           VARCHAR(16)  NOT NULL DEFAULT 'viewer',
    department_id  BIGINT REFERENCES departments (id) ON DELETE RESTRICT,
    status         VARCHAR(16)  NOT NULL DEFAULT 'active',
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT admin_users_role_chk CHECK (role IN ('super_admin', 'dept_admin', 'viewer')),
    CONSTRAINT admin_users_dept_chk CHECK (role = 'super_admin' OR department_id IS NOT NULL)
);
CREATE INDEX idx_admin_users_department ON admin_users (department_id);

-- backfill departments from the free-text column, then replace it with a FK
ALTER TABLE applications ADD COLUMN department_id BIGINT REFERENCES departments (id) ON DELETE RESTRICT;
INSERT INTO departments (code, name)
SELECT 'dept-' || row_number() OVER (ORDER BY department), department
FROM (SELECT DISTINCT department FROM applications WHERE department <> '') d;
UPDATE applications a SET department_id = d.id FROM departments d WHERE d.name = a.department;
ALTER TABLE applications DROP COLUMN department;
CREATE INDEX idx_applications_department ON applications (department_id);

-- "my models" become per user; legacy rows are adopted by the bootstrap admin
ALTER TABLE my_models ADD COLUMN user_id BIGINT;
ALTER TABLE my_models DROP CONSTRAINT IF EXISTS my_models_model_id_key;
CREATE UNIQUE INDEX uq_my_models_user_model ON my_models (user_id, model_id);

-- alerts carry the API key they concern so they can be scoped to a department
ALTER TABLE alert_events ADD COLUMN key_id BIGINT;
CREATE INDEX idx_alert_events_key_id ON alert_events (key_id);
