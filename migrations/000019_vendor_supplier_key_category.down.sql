DROP INDEX IF EXISTS idx_api_keys_holder;
DROP INDEX IF EXISTS idx_api_keys_category;
DROP INDEX IF EXISTS idx_api_keys_department;
ALTER TABLE api_keys DROP COLUMN IF EXISTS allowed_models;
ALTER TABLE api_keys DROP COLUMN IF EXISTS coding_tools;
ALTER TABLE api_keys DROP COLUMN IF EXISTS employee_no;
ALTER TABLE api_keys DROP COLUMN IF EXISTS holder_user_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS department_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS category;
UPDATE model_routes r SET source_vendor_id = p.id FROM vendors v JOIN providers p ON p.code = v.code
WHERE r.source_vendor_id = v.id;
DROP TABLE IF EXISTS vendor_suppliers;
DROP INDEX IF EXISTS idx_models_vendor;
ALTER TABLE models DROP COLUMN IF EXISTS vendor_id;
ALTER TABLE providers DROP COLUMN IF EXISTS contact;
ALTER TABLE providers DROP COLUMN IF EXISTS supplier_type;
DROP TABLE IF EXISTS vendors;
