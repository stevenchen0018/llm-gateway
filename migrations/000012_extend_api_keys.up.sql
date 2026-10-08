ALTER TABLE api_keys ADD COLUMN app_id           BIGINT;
ALTER TABLE api_keys ADD COLUMN key_type         VARCHAR(16) NOT NULL DEFAULT 'formal'; -- formal | trial
ALTER TABLE api_keys ADD COLUMN owner_email      VARCHAR(256) NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN manager          VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN manager_email    VARCHAR(256) NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN blacklist_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN blacklisted_at   TIMESTAMPTZ;
CREATE INDEX idx_api_keys_app_id ON api_keys (app_id);
