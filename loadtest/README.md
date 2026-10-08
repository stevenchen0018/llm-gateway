# loadtest — TPM / QPS 压测工具

独立的 Go 程序，向网关的 OpenAI 兼容接口施压，报告吞吐、延迟与限流表现，并可**断言限流是否准确**（CI 可用：断言失败退出码为 3）。

```bash
go build -o bin/loadtest ./loadtest        # 或 make loadtest-build
bin/loadtest -h
```

## 模式

| `-mode` | 说明 | 关键参数 |
|---|---|---|
| `qps` | 开环、恒定请求速率（不受响应快慢影响，能真实压出限流） | `-qps` |
| `tpm` | 开环，按「目标 TPM ÷ 实测平均 tokens/请求」每秒自适应调整速率 | `-tpm` |
| `ramp` | 阶梯加压，直到某一阶梯失败/限流比例超过 `-stop-ratio`，用于探测容量上限 | `-ramp-start -ramp-step -ramp-interval -ramp-max` |
| `concurrency` | 闭环，N 个并发连续发送 | `-concurrency` |

通用参数：`-url`（默认 `http://localhost:8080`）、`-key`（可逗号分隔多个，轮询使用；也可用环境变量 `LLMGW_API_KEY`）、`-model`、`-endpoint chat|completions|embeddings`、`-prompt-tokens`（提示词大小）、`-max-tokens`（同时决定网关的 TPM 预占量）、`-duration`、`-max-inflight`、`-timeout`。

输出：每秒一行进度（`-quiet` 关闭），结束时打印报告；`-out report.json` 保存完整报告，`-csv timeline.csv` 保存逐秒时间线。

## 限流断言

- `-expect-qps N`：任意 1 秒窗口内**成功**的请求数不得超过 `N × (1 + tolerance) + 1`（+1 是客户端按发送时间、网关按检查时间分桶造成的边界误差）。
- `-expect-tpm N`：任意自然分钟内成功请求的 tokens 不得超过 `N × (1 + tolerance)`。至少跑满 1 分钟才有意义。
- `-tolerance`（默认 0.1）。

## 场景（使用 `make seed` 的演示数据）

```bash
# 1) Key 级 QPS：试用 Key 配额 QPS=2，以 10/s 施压，应稳定在 2/s，其余 429
bin/loadtest -key sk-demo-a2-customer-test -model doubao-flash -qps 10 -duration 10s -expect-qps 2

# 2) Key 级 TPM：同一 Key 的 TPM=20000，以 60000 TPM 施压 2 分钟
bin/loadtest -key sk-demo-a2-customer-test -model doubao-flash -mode tpm -tpm 60000 -duration 130s -expect-tpm 20000

# 3) 部门级 QPS：先在「部门管理」把客户体验部 QPS 设为 5，再用该部门的两个 Key 合计 20/s 施压
bin/loadtest -key sk-demo-a1-customer-service,sk-demo-a2-customer-test -model doubao-flash -qps 20 -duration 10s -expect-qps 5

# 4) 容量探测：从 20 QPS 起每 5 秒 +20，直到 30% 请求失败/被限流
bin/loadtest -key sk-demo-a1-customer-service -model doubao-flash -mode ramp -ramp-start 20 -ramp-step 20 -ramp-max 400 -ramp-interval 5s -stop-ratio 0.3

# 5) 并发压测 + 导出
bin/loadtest -key sk-demo-a1-customer-service -model doubao-flash -mode concurrency -concurrency 50 -duration 30s -out report.json -csv timeline.csv
```

## 解读

- **429 rate_limited** 表示命中 Key、部门或模型的 QPS/TPM 上限（模型容量饱和时网关会先切换到下一个候选，全部饱和才 429）。
- **TPM 是自然分钟固定窗口**：跨分钟边界的短时间内可能出现接近 2 倍配额的突发，因此平均 TPM 在非整分钟时长下可能高于配额，以「单分钟峰值」为准。
- 网关对 TPM 按 `输入估算 + max_tokens` 预占、按实际用量结算，`-max-tokens` 设得越大，同样配额下可并发的请求越少。
- **客户端丢弃**（dropped）> 0 说明压测机自身达到 `-max-inflight` 上限，结果偏保守，应调大该值或分布式施压。
- 演示环境使用 Mock 厂商，延迟主要是网关自身开销；对接真实厂商时延迟与失败率以厂商为准。
