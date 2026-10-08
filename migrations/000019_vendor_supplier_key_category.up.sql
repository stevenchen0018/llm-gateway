-- 厂商 (model vendor: DeepSeek, 阿里通义, OpenAI ...). The existing providers
-- table is the 供应商 (supply channel: official API, cloud platform,
-- reseller, self-hosted) through which a vendor's models are bought.
CREATE TABLE vendors (
    id                BIGSERIAL PRIMARY KEY,
    code              VARCHAR(32) NOT NULL UNIQUE,
    name              VARCHAR(64) NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    website           VARCHAR(256) NOT NULL DEFAULT '',
    -- how requests for this vendor's models are spread over its suppliers
    -- when no scheduling policy matches: cost_first | supplier_priority | supplier_weighted
    routing_strategy  VARCHAR(24) NOT NULL DEFAULT 'cost_first',
    status            VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE providers ADD COLUMN supplier_type VARCHAR(16) NOT NULL DEFAULT 'cloud'; -- official | cloud | reseller | self_hosted
ALTER TABLE providers ADD COLUMN contact VARCHAR(128) NOT NULL DEFAULT '';

ALTER TABLE models ADD COLUMN vendor_id BIGINT REFERENCES vendors (id) ON DELETE SET NULL;
CREATE INDEX idx_models_vendor ON models (vendor_id);

-- which suppliers supply a vendor, with per-supplier scheduling settings
CREATE TABLE vendor_suppliers (
    id           BIGSERIAL PRIMARY KEY,
    vendor_id    BIGINT NOT NULL REFERENCES vendors (id) ON DELETE CASCADE,
    provider_id  BIGINT NOT NULL REFERENCES providers (id) ON DELETE CASCADE,
    priority     INTEGER NOT NULL DEFAULT 100,  -- lower first (supplier_priority)
    weight       INTEGER NOT NULL DEFAULT 100,  -- share (supplier_weighted)
    status       VARCHAR(16) NOT NULL DEFAULT 'active', -- disabled = never routed to
    remark       TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vendor_id, provider_id)
);

-- existing databases: one vendor per current provider keeps everything routable
INSERT INTO vendors (code, name, description) SELECT code, name, description FROM providers ON CONFLICT (code) DO NOTHING;
UPDATE models m SET vendor_id = v.id FROM providers p JOIN vendors v ON v.code = p.code WHERE m.provider_id = p.id;
INSERT INTO vendor_suppliers (vendor_id, provider_id) SELECT v.id, p.id FROM providers p JOIN vendors v ON v.code = p.code
ON CONFLICT DO NOTHING;
-- "厂商模型" policy sources pointed at providers; they now point at vendors
UPDATE model_routes r SET source_vendor_id = v.id FROM providers p JOIN vendors v ON v.code = p.code
WHERE r.source_vendor_id = p.id;

-- Key categories: application (online systems) vs personal (employee daily coding)
ALTER TABLE api_keys ADD COLUMN category VARCHAR(16) NOT NULL DEFAULT 'application';
-- the owning department, stored on the key (personal keys have no application)
ALTER TABLE api_keys ADD COLUMN department_id BIGINT REFERENCES departments (id) ON DELETE SET NULL;
-- the console user who owns a personal key (self-service apply / claim)
ALTER TABLE api_keys ADD COLUMN holder_user_id BIGINT REFERENCES admin_users (id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD COLUMN employee_no VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN coding_tools TEXT NOT NULL DEFAULT '[]';   -- JSON []string
ALTER TABLE api_keys ADD COLUMN allowed_models TEXT NOT NULL DEFAULT '[]'; -- JSON []string, empty = all
UPDATE api_keys k SET department_id = a.department_id FROM applications a WHERE a.id = k.app_id;
CREATE INDEX idx_api_keys_department ON api_keys (department_id);
CREATE INDEX idx_api_keys_category ON api_keys (category);
CREATE INDEX idx_api_keys_holder ON api_keys (holder_user_id);
