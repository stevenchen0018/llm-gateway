DROP TABLE IF EXISTS provider_test_results;
DROP TABLE IF EXISTS provider_test_runs;
ALTER TABLE providers DROP COLUMN IF EXISTS protocol;
