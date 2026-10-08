DROP INDEX IF EXISTS idx_model_routes_api_key_id;
ALTER TABLE model_routes DROP COLUMN remark, DROP COLUMN source_vendor_id, DROP COLUMN source_type, DROP COLUMN api_key_id, DROP COLUMN app_id, DROP COLUMN name;
