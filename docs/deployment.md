# 部署指南

本文介绍三种部署方式，参考了 [New API 集群部署](https://docs.newapi.ai/zh/docs/installation/deployment-methods/cluster-deployment) 的主从思路：

| 方式 | 适用场景 | 文件 |
|---|---|---|
| Docker 单容器 | 已有 PostgreSQL / Redis，只想跑网关 | `deployments/Dockerfile` |
| Docker Compose 单机 | 试用、测试环境、小规模生产 | `deployments/docker/` |
| 集群（主从 + 负载均衡） | 生产、高可用、水平扩容 | `deployments/cluster/`（单机多节点）、`deployments/cluster/multi-host/`（多主机） |

镜像同时包含 **网关 API** 和 **控制台**（`web/dist` 由网关直接托管，带 SPA 回退）。所以一个端口就同时提供控制台（`/`）、OpenAI 兼容 API（`/v1`）和管理 API（`/admin/v1`）。

---

## 1. 构建镜像

```bash
# 在仓库根目录
make docker-build                 # 等价于 docker build -f deployments/Dockerfile -t llm-gateway:latest .
make docker-build IMAGE=registry.example.com/llm-gateway:1.0.0 && docker push registry.example.com/llm-gateway:1.0.0
```

构建分三阶段：

1. `node:22` 构建控制台。
2. `golang:1.26` 编译 `gateway`、`migrate`、`seed` 三个静态二进制。
3. 产出 `alpine` 运行镜像，约 80 MB，以非 root 用户（uid 10001）运行，自带 `HEALTHCHECK`。

国内网络可以加 `--build-arg GOPROXY=https://goproxy.cn,direct`。

## 2. Docker 单容器

```bash
docker run -d --name llm-gateway --restart unless-stopped -p 8080:8080 \
  -e LLM_GATEWAY_POSTGRES_HOST=10.0.0.10 -e LLM_GATEWAY_POSTGRES_PASSWORD=*** \
  -e LLM_GATEWAY_REDIS_ADDR=10.0.0.10:6379 -e LLM_GATEWAY_REDIS_PASSWORD=*** \
  -e LLM_GATEWAY_JWT_SECRET=$(openssl rand -hex 32) \
  -e LLM_GATEWAY_ADMIN_PASSWORD='首个超管密码' \
  llm-gateway:latest
```

所有配置都可以用环境变量 `LLM_GATEWAY_<段>_<字段>` 覆盖，完整清单见 `configs/config.yaml.example`。也可以挂载配置文件：

```bash
-v $PWD/config.yaml:/app/config.yaml  ...  llm-gateway:latest -config /app/config.yaml
```

## 3. Docker Compose 单机

```bash
cd deployments/docker
cp .env.example .env && vi .env          # 修改所有 [必改] 项
docker compose up -d --build
docker compose --profile demo run --rm seed   # 可选：灌入演示数据（仅空库）
```

- 包含 PostgreSQL 17、Redis 7 和网关；数据保存在命名卷 `postgres-data`、`redis-data` 中。
- 打开 `http://<host>:8080`，用 `.env` 里的 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 登录。首次登录后请改密码。
- 升级：`docker compose build gateway && docker compose up -d gateway`。数据库迁移在启动时自动执行。

> 本地开发仍用 `make dev-up`（`deployments/docker-compose.yml`）。它只启动 Postgres + Redis，网关用 `make run`、控制台用 `make web-dev`。

---

## 4. 集群部署

### 4.1 架构

```
                       ┌──────────── Nginx / SLB（least_conn，剔除故障节点）────────────┐
                       ▼                              ▼                               ▼
               gw-master (master)               gw-slave-1 (slave)   …         gw-slave-N (slave)
          迁移 / 初始化超管 / 定时任务              无状态副本                          无状态副本
                       └──────────────┬───────────────┴───────────────┬───────────────┘
                                      ▼                               ▼
                       PostgreSQL（唯一数据源，建议 RDS 主备）     Redis（必须共享：限流 / 熔断 / 任务锁）
```

所有节点都处理完整的 API（网关调用、控制台、管理 API）。master 和 slave 的区别只在于谁维护共享状态：

| 职责 | master | slave |
|---|---|---|
| 处理 `/v1`、`/admin/v1`、控制台 | ✅ | ✅ |
| 启动时执行数据库迁移 | ✅ | ❌（等待 master 完成，最长 `cluster.schema_wait`） |
| 首次启动初始化超级管理员 | ✅ | ❌ |
| 每周成本周报推送、请求记录过期清理 | ✅（另有 Redis 任务锁兜底，部署了多个 master 也只执行一次） | ❌ |
| 用量 / 请求记录异步落库、过滤规则命中计数 | ✅（各节点写各自处理的请求，累加写入，不冲突） | ✅ |

**和 New API 的关键区别**：本网关的 Key / 模型 TPM、QPS 限流、供应商熔断健康状态、定时任务锁都保存在 Redis 中。因此**所有节点必须连接同一个 Redis**，不支持“每个节点各自一个 Redis”，否则限流会按节点数放大，故障切换状态也不一致。

各节点的配置缓存（部门状态 30 秒，系统设置 5 秒，过滤规则 10 秒）都会到期后从数据库刷新。在一个节点上修改配置，其他节点最多 30 秒内生效，相当于 New API 的 `SYNC_FREQUENCY`。

### 4.2 关键配置

| 环境变量 | 说明 | 要求 |
|---|---|---|
| `LLM_GATEWAY_CLUSTER_NODE_TYPE` | `master` / `slave`，默认 `master` | 集群内**只部署 1 个 master** |
| `LLM_GATEWAY_CLUSTER_NODE_NAME` | 节点名，默认取主机名（容器 ID）；会写入日志和响应头 `X-Gateway-Node` | 建议每个节点唯一 |
| `LLM_GATEWAY_CLUSTER_SCHEMA_WAIT` | slave 启动时等待 master 完成迁移的最长时间，默认 `120s` | |
| `LLM_GATEWAY_POSTGRES_*` | 数据库连接 | 所有节点相同 |
| `LLM_GATEWAY_POSTGRES_MAX_OPEN_CONNS` | 单节点连接池上限 | 节点数 × 该值 < PostgreSQL `max_connections` |
| `LLM_GATEWAY_REDIS_ADDR` / `_PASSWORD` / `_DB` | Redis 连接（单一地址） | 所有节点**必须相同** |
| `LLM_GATEWAY_JWT_SECRET` | 控制台登录令牌签名密钥（相当于 New API 的 `SESSION_SECRET`） | 所有节点**必须相同**，否则登录后请求打到别的节点会 401 |
| `LLM_GATEWAY_SERVER_TRUSTED_PROXIES` | 负载均衡的 IP / 网段，逗号分隔 | 只填 LB，否则调用方能伪造来源 IP，绕过 Key IP 白名单 |
| `LLM_GATEWAY_SERVER_WEB_ROOT` | 控制台静态文件目录，镜像内默认 `/app/web`；置空则只提供 API | |

### 4.3 单机多节点（`deployments/cluster/`）

适合演示、压测，或者单台大机器想用满多核并获得进程级容错的场景。

```bash
cd deployments/cluster
cp .env.example .env && vi .env
docker compose up -d --build                       # nginx + 1 master + 2 slave + postgres + redis
docker compose up -d --scale gateway-slave=4       # 扩容；Nginx 通过 Docker DNS 10 秒内自动发现新副本
docker compose exec gateway-master seed            # 可选：演示数据
```

- 入口是 `http://<host>:${LB_PORT}`。Nginx 使用 `least_conn` 均衡（大模型请求耗时差异大，按最少连接比轮询更均匀），master 权重 3、slave 权重 5。
- 只有 Nginx 暴露端口，网关节点只信任集群网段 `CLUSTER_SUBNET` 传来的 `X-Forwarded-For`。
- 可以用响应头 `X-Gateway-Node` 确认请求由哪个节点处理；Nginx 访问日志也会记录 `upstream` 和 `node`。

### 4.4 多主机（`deployments/cluster/multi-host/`）

与 New API 文档的做法一致：每台机器放一份 `.env`（使用同一套密钥），按机器角色启动对应的 compose 文件。

| 机器 | 命令 | 说明 |
|---|---|---|
| 数据机（可选） | `docker compose -f infra.compose.yml up -d` | 自建 PostgreSQL + Redis；生产建议用云 RDS + 云 Redis |
| 主节点 ×1 | `docker compose -f master.compose.yml up -d` | `.env` 里 `NODE_NAME=gw-master` |
| 从节点 ×N | `docker compose -f slave.compose.yml up -d` | 每台机器的 `NODE_NAME` 不同 |
| 入口机 | 使用 `nginx.conf`（放到 `/etc/nginx/conf.d/`） | 把 upstream 改成各节点 IP；建议启用 HTTPS |

镜像先推送到私有仓库（`GATEWAY_IMAGE`），各节点再 `docker compose pull`。

**扩容步骤**：

1. 准备新机器并安装 Docker。
2. 复制 `.env`，修改 `NODE_NAME`。
3. 执行 `docker compose -f slave.compose.yml up -d`。
4. 确认 `curl http://<新节点>:8080/readyz` 返回 `ready`。
5. 在入口 Nginx 的 upstream 中加入新节点，然后执行 `nginx -s reload`。

使用云 SLB 时，健康检查请配置为 `GET /readyz`，返回 200 视为健康。它会同时检查数据库、Redis 和 schema 版本；`/healthz` 只表示进程存活。

### 4.5 滚动升级

1. **先升级 master**：`docker compose -f master.compose.yml pull && docker compose -f master.compose.yml up -d`。master 启动时执行数据库迁移；多个进程同时迁移时，由 PostgreSQL advisory lock 串行化。
2. 确认 master 的 `/readyz` 返回 `ready`，并且日志里有 `database schema ready`。
3. **逐台升级 slave**，每台都等 `/readyz` 恢复后再升级下一台。新版本的 slave 如果先于 master 启动，会打印 `waiting for master to migrate the schema` 并等待，不会带着旧 schema 对外服务。
4. 网关收到 `SIGTERM` 后会优雅退出：停止接收新连接，等待进行中的请求完成，并把缓冲的用量和请求记录写入数据库（`stop_grace_period: 30s`）。

### 4.6 Nginx 要点

完整配置见 `deployments/cluster/nginx/nginx.conf` 和 `multi-host/nginx.conf`。

- `proxy_buffering off` 和 `proxy_read_timeout 300s`：支持 SSE 流式输出和长时间推理。
- `proxy_set_header X-Forwarded-For $remote_addr`：当 Nginx 是边缘入口时，用真实对端地址覆盖客户端自带的 XFF，防止伪造来源 IP。如果 Nginx 前面还有 SLB，请改用 `real_ip` 模块，并在 `set_real_ip_from` 中填写 SLB 网段。
- `proxy_next_upstream error timeout`：只在连接失败时换节点重试。POST 请求一旦发出就不会被重放，避免重复调用厂商和重复计费。
- `client_max_body_size 50m`：支持多模态请求和模板批量导入。

---

## 5. 运维与排障

| 现象 | 排查 |
|---|---|
| 登录后操作偶尔 401 | 各节点的 `JWT_SECRET` 不一致 |
| 限流阈值好像被放大了 N 倍 | 节点没有连接同一个 Redis（检查 `REDIS_ADDR` / `REDIS_DB`） |
| Key IP 白名单失效，或日志来源 IP 都是 LB 地址 | `TRUSTED_PROXIES` 没配置或配置错误；Nginx 没有设置 `X-Forwarded-For` |
| slave 启动失败：`start or upgrade the master node first` | master 没启动，或 master 的版本旧于 slave。请先升级 master |
| `too many connections` | 调小 `PG_MAX_OPEN_CONNS`，或调大 PostgreSQL `max_connections` |
| 负载不均 | 查看 Nginx 日志中的 `upstream` / `node` 分布，调整 `weight`；确认没有节点被 `max_fails` 摘除 |

- **监控**：每个节点都暴露 `/metrics`（Prometheus），请按节点分别抓取，不要经过 LB。另外需要关注 CPU、内存、PostgreSQL 连接数和 Redis 内存。
- **备份**：集群部署也需要定期备份 PostgreSQL，例如 `pg_dump` 或 RDS 自动备份。Redis 中只保存限流窗口、熔断状态和任务锁，丢失后会自动重建，无需备份。
- **Redis 拓扑**：网关使用单一 Redis 地址，不直接支持 Redis Cluster / Sentinel 协议。请使用提供统一访问地址的云 Redis（主从）或代理。
