DROP INDEX IF EXISTS idx_api_keys_app_id;
ALTER TABLE api_keys DROP COLUMN blacklisted_at, DROP COLUMN blacklist_reason, DROP COLUMN manager_email, DROP COLUMN manager, DROP COLUMN owner_email, DROP COLUMN key_type, DROP COLUMN app_id;
