DROP INDEX IF EXISTS idx_usage_records_created_at;
ALTER TABLE usage_records DROP COLUMN list_cost;
DROP TABLE IF EXISTS metrics_minute;
