DROP TABLE IF EXISTS request_logs;
DROP TABLE IF EXISTS content_filter_rules;
DROP TABLE IF EXISTS gateway_settings;
ALTER TABLE api_keys DROP COLUMN IF EXISTS ip_whitelist;
