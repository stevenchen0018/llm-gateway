-- scheduling policy (调度策略): (application, api key, source model) -> target model
ALTER TABLE model_routes ADD COLUMN name             VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE model_routes ADD COLUMN app_id           BIGINT;
ALTER TABLE model_routes ADD COLUMN api_key_id       BIGINT;
ALTER TABLE model_routes ADD COLUMN source_type      VARCHAR(16) NOT NULL DEFAULT 'custom';
ALTER TABLE model_routes ADD COLUMN source_vendor_id BIGINT;
ALTER TABLE model_routes ADD COLUMN remark           TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_model_routes_api_key_id ON model_routes (api_key_id);
