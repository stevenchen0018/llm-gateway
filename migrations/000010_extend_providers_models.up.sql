-- vendor discount (厂商折扣): effective price = list price * discount_rate
ALTER TABLE providers ADD COLUMN discount_rate NUMERIC(6, 4) NOT NULL DEFAULT 1;
ALTER TABLE providers ADD COLUMN description TEXT NOT NULL DEFAULT '';

-- model marketplace attributes
ALTER TABLE models ADD COLUMN category       VARCHAR(32) NOT NULL DEFAULT 'text';
ALTER TABLE models ADD COLUMN context_length INTEGER NOT NULL DEFAULT 0;
ALTER TABLE models ADD COLUMN tags           TEXT NOT NULL DEFAULT '[]';
ALTER TABLE models ADD COLUMN description    TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN released_at    DATE;
CREATE INDEX idx_models_category ON models (category);
