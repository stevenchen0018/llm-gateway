-- 接口参数适配规则：按 协议 / 供应商 / 模型 / 接口 匹配，改写发往供应商的请求体
CREATE TABLE param_rules (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(128) NOT NULL,
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    source         VARCHAR(16) NOT NULL DEFAULT 'manual',   -- builtin | manual | auto
    protocol       VARCHAR(16) NOT NULL DEFAULT '',         -- '' = 任意协议
    provider_id    BIGINT REFERENCES providers(id) ON DELETE CASCADE,  -- NULL = 任意供应商
    model_pattern  VARCHAR(512) NOT NULL DEFAULT '',        -- 逗号分隔的通配符，匹配上游模型名；'' = 任意
    op             VARCHAR(16) NOT NULL DEFAULT '',         -- chat | embedding | image | video | ''
    actions        JSONB NOT NULL DEFAULT '[]',
    priority       INTEGER NOT NULL DEFAULT 100,
    remark         TEXT NOT NULL DEFAULT '',
    hit_count      BIGINT NOT NULL DEFAULT 0,
    last_hit_at    TIMESTAMPTZ,
    created_by     VARCHAR(64) NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 每个 供应商 + 模型 + 接口 最多一条自动学习规则（多节点同时学习时去重）
CREATE UNIQUE INDEX uq_param_rules_auto ON param_rules (provider_id, model_pattern, op) WHERE source = 'auto';

INSERT INTO param_rules (name, enabled, source, protocol, model_pattern, op, actions, priority, remark, created_by) VALUES
('OpenAI 新一代模型：max_tokens → max_completion_tokens', TRUE, 'builtin', 'openai', 'o1*,o3*,o4*,gpt-5*,gpt-6*', 'chat',
 '[{"type":"rename","field":"max_tokens","to":"max_completion_tokens"}]', 100,
 'OpenAI 推理模型与新一代模型不再接受 max_tokens。尚未升级的渠道（如旧版 Azure api-version、部分代理商）若报错，网关会自动改回 max_tokens 并为该供应商记录自动规则；也可为该供应商新增一条手动规则覆盖本规则。', 'system'),
('OpenAI 推理模型：去掉采样参数', FALSE, 'builtin', 'openai', 'o1*,o3*,o4*', 'chat',
 '[{"type":"drop","field":"temperature"},{"type":"drop","field":"top_p"}]', 110,
 '推理模型只接受默认的 temperature / top_p。默认关闭：供应商报错时网关会自动去掉这些参数并记录；如需提前统一去掉可开启。', 'system');
