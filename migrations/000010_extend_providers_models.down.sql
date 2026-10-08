DROP INDEX IF EXISTS idx_models_category;
ALTER TABLE models DROP COLUMN released_at, DROP COLUMN description, DROP COLUMN tags, DROP COLUMN context_length, DROP COLUMN category;
ALTER TABLE providers DROP COLUMN description, DROP COLUMN discount_rate;
