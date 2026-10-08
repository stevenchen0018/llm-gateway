# 架构与功能对照

本文对照设计截图说明控制台与网关的实现。

## 1. 技术架构：厂商匹配 → 模型调度 → 厂商服务代理

```
业务域 ──调用统一 API（携带模型名 model）──▶ 网关
   ① 厂商匹配   model 名 → 已上架模型 → 厂商           RoutingService.Explain (step 1)
   ② 模型调度   API Key + 模型 + 厂商 → 命中的调度策略   RoutingService.Explain (step 2)
                  Key 专属策略 ＞ 全局策略 ＞ 直连（按折后单价排序）
   ③ 厂商服务代理  按顺序调用目标；异常/不健康自动切换    GatewayService + ProviderClient + HealthStore
   ──▶ 阿里 / 字节 / 百度 / 华为 / 谷歌 / 微软 / DeepSeek / KubeAI(自建) …
```

- 请求里的 `model` 可以写 `deepseek-r1`，也可以写 `aliyun/deepseek-r1` 固定源厂商。
- 控制台「模型调度」页顶部的架构图即上述链路；「路由模拟」使用与真实请求完全相同的解析逻辑并逐级高亮。
- 调度策略 = (应用, API Key, 源模型[自定义 | 厂商模型]) → 有序目标（厂商 + 模型），
  策略可选 `priority` / `cost_first`（折后单价）/ `round_robin`（加权）。表：`model_routes`。

## 2. 分钟级监控（对应 Flink 实时计算）

网关每次调用由 `AsyncUsageWorker` 异步写入 `usage_records`（原始调用日志），同时折叠进
分钟级汇总表 `metrics_minute(bucket, key_id, model_id)`（每秒批量 UPSERT）。所有监控、成本、
容量看板都读这张表：

| 页面 | 接口 | 指标 |
|---|---|---|
| Key 监控（基于 Key） | `GET /admin/v1/monitor/timeseries?group_by=model&key_id=` | RPM、TPM、平均 RT、Token 趋势（总量/输入/输出）、调用趋势（总调用/失败数/失败率） |
| Key 监控 · 排行 | `GET /admin/v1/monitor/ranking` | 调用失败率、平均 RT（按模型） |
| 模型监控（基于模型） | 同上，`provider_id` / `model_id` 过滤 | 调用监控、性能监控（TPM / RPM / RT） |
| 容量管理 | `GET /admin/v1/capacity` | Key 与模型 TPM/QPS 配额 vs 近 1 小时峰值 |

- `RPM = requests / (step/60)`，`TPM = tokens / (step/60)`，`RT = Σlatency / Σrequests`。
- 时间粒度按范围自动选取（≤720 个点），前端补齐空桶：计数类补 0，比率类断线。
- 多序列图最多 7 个实体 + 灰色「其他」，实体颜色跨筛选保持稳定。

## 3. 降本五步 → 功能

| 步骤 | 功能 |
|---|---|
| ① 源头管控 | Key 申请工单 + 试用 Key（7 天、小配额）；预算申请并**分级审批**：≤ `budget.director_limit` 总监(D)，超过 CTO；通过后才绑定 Key 并被网关管控 |
| ② 成本感知 | 预算预警（阈值/耗尽告警）、成本大盘（应用/Key/模型/厂商/类别）、每周一 09:00 向使用人及 +1 主管推送周报（也可手动推送） |
| ③ 模型切换 | 统一 API + 调度策略；「均价赛马 · 模型切换建议」列出同类别更低价模型及预计月度节省，一键预填调度策略 |
| ④ 厂商折扣 | 厂商 `discount_rate`；成本 = 标价 × 折扣，`usage_records/metrics_minute` 同时保留标价成本，可看折扣节省 |
| ⑤ 均价赛马 / 成本大盘 | 按模型类别对比每百万 Token 均价（实测折后价优先，无调用则用配置价） |

## 4. 控制台菜单

概览 · 模型（市场 / 我的模型 / 调度）· 模型体验（文本生成 / 图片理解 / 图片生成）·
监控与容量（模型监控 / Key 监控 / 容量管理 / 告警 / 调用日志）· 模型成本（成本大盘 / 均价赛马 / 预算）·
API Key（申请 / 管理 / 黑名单）· 平台管理（部门 / 用户 / 应用 / 厂商 / 公告 / 审计）· 开发者（API 文档）。

## 4.1 多租户与权限

```
部门(departments) ─┬─ 应用(applications.department_id) ── API Key ─┬─ 预算
                   │                                             ├─ Key 专属调度策略
                   └─ 成员(admin_users: dept_admin / viewer)      └─ 用量 / 告警(key_id) / 审计
```

- 超级管理员管理平台级资源（厂商、模型、全局策略、公告、部门、所有用户），并可按部门切换视角。
- 部门管理员只能操作本部门的应用、Key、预算（D 级审批；CTO 级需超级管理员）、Key 专属策略与成员；
  不能授予超级管理员、不能修改自己的角色/部门/状态；最后一个启用中的超级管理员不能被降级或停用。
- 只读成员对本部门数据只读。越权访问一律 403，由服务端（`internal/api/admin/helpers.go` 的
  `deptScope` / `ensureOwns`）强制，前端菜单隐藏只是体验优化。
- JWT 只携带用户 ID，角色与部门每次请求从库加载，禁用/改角色即时生效。

## 4.2 限流的三个维度

| 维度 | Redis key | 共享范围 | 超限表现 |
|---|---|---|---|
| Key | `rl:qps:{key}:0:*` / `rl:tpm:{key}:0:*` | 该 Key 的全部模型 | 429 |
| 部门 | `rl:qps:dept:{dept}:*` / `rl:tpm:dept:{dept}:*` | 部门内全部 Key | 429 |
| 模型 | `rl:qps:0:{model}:*` / `rl:tpm:0:{model}:*` | 所有 Key | 切换下一个候选，全部饱和才 429 |

最初实现把 Key 配额按 (Key, 模型) 计，故障切换时每个候选都会再给一份额度——`loadtest` 在 QPS=2
的 Key 上测出了 4/s 的成功率，因此拆成了上面三个独立维度，并由 `TestKeyAndModelScopesAreIndependent` 覆盖。

## 4.3 API 文档与压测

- `GET /openapi.json`：OpenAPI 3.0 规范（`internal/api/docs/openapi.json`，嵌入二进制），控制台「API 文档」页基于它渲染并支持在线调试。
- `loadtest/`：独立的 TPM/QPS 压测程序，见 [loadtest/README.md](../loadtest/README.md)。

## 5. 演示数据

`make seed-reset` 生成 8 个厂商、32 个模型、5 个应用、10 个 Key（正式/试用/待审批/已拉黑）、
7 个预算（含待审批 D/CTO、已驳回、已耗尽）、12 条调度策略、**14 天分钟级流量**（含日夜/周末形态，
以及 `doubao-1-5-thinking-vision-pro` 最近 100 分钟的 RT 飙升 + 失败率上升事故）、3000 条调用日志、告警与审计。
数据相对「当前时间」生成；隔一段时间后想让曲线右端重新对齐，再执行一次 `make seed-reset`。

## 4.4 安全与合规

```
/v1 请求 ─▶ RequestCapture（分配 X-Request-Id、挂 RequestTrace、按设置截取请求/响应体）
         ─▶ APIKeyAuth（Key 校验 → IP 白名单，拒绝则 403 + 安全告警）
         ─▶ GatewayService：预算 → 提示词过滤（拦截 / 脱敏 / 记录）→ 限流 → 路由调用 → 输出过滤
         ◀─ RequestCapture 汇总 Trace（命中模型、Token、命中规则）→ RequestLogWorker 异步批量入库
```

- 运行时开关存于 `gateway_settings`（5 秒缓存），规则存于 `content_filter_rules`（10 秒缓存，编辑后本实例立即刷新）。
  关键词规则会被编译为一个不区分大小写的 RE2 正则，所有规则都是线性时间匹配，不存在回溯爆炸。
- 脱敏发生在转发厂商之前（单测 `TestGatewayMasksPromptBeforeUpstreamAndBlocks` 用回显型 Mock 厂商验证厂商收不到原文）；
  存档内容可再按脱敏规则处理一次，避免请求记录本身成为敏感数据泄露点。
- 视频生成是异步任务：`VideoClient` 接口（提交 / 查询 / 取内容），OpenAI 兼容适配器对接 `/videos`，
  Mock 适配器约 8 秒完成并返回动态预览。

## 4.5 厂商、供应商与两类 Key

```
厂商 vendors ──< vendor_suppliers(priority, weight, status) >── 供应商 providers(base_url, 凭证, 折扣, supplier_type)
   │                                                               │
   └──────────────< models(vendor_id, provider_id, 价格, TPM/QPS) >──┘

请求 model=deepseek-v3 → 匹配 DeepSeek 的全部供应商上架的 deepseek-v3（去掉停用的供应商链接）
   → Key 专属策略 / 全局策略（可按供应商优先级或权重排序目标）
   → 无策略：按厂商 routing_strategy（cost_first | supplier_priority | supplier_weighted）排序供应商
   → 依次调用，失败 / 熔断 / 模型容量饱和时切换下一家供应商
```

- `api_keys.category`：`application`（归属应用，部门取自应用，应用迁移部门时 Key 随之迁移）与 `personal`（归属员工与部门，
  `holder_user_id` 指向控制台账号）。部门归属统一存于 `api_keys.department_id`，所有部门过滤与统计都基于它。
- 个人编码 Key 的密钥由持有人领取（`/keys/:id/claim`），审批人只完成激活；`allowed_models` 在网关入口校验。
- 监控与成本新增维度：`group_by=vendor`（厂商）、`group_by=key_category`（Key 类型），过滤参数 `vendor_id`、`key_category`。
