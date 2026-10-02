# 在 Render 免费容器上部署 sub2api（外部 PostgreSQL + Redis）

本文说明如何用仓库根目录的 **`render.yaml`** 蓝图，把 sub2api 部署到 **Render 免费容器实例（美国区域）**，
数据库与 Redis 使用**外部已存在的 Aiven 实例**（PostgreSQL + Valkey）。

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/countossbot/sub2api)

> 免费实例**没有持久磁盘**：`/app/data` 里的 `config.yaml` 会在每次重新部署时丢失。
> 本项目已支持「无磁盘启动」——只要环境变量齐全，应用会自动重建配置，且不会因为数据库里已存在管理员而启动失败。

---

## 1. 环境变量（共 4 个）

只需以下 4 个变量。**不需要**任何 `DATABASE_HOST` / `DATABASE_PORT` / `DATABASE_USER` /
`DATABASE_PASSWORD` / `DATABASE_NAME` / `DATABASE_SSLMODE` 或 `REDIS_HOST` / `REDIS_PORT` /
`REDIS_PASSWORD` / `REDIS_ENABLE_TLS` 分项变量——两个连接串已包含全部连接信息。

| 变量 | 必需 | 作用 | 来源 / 格式 |
| --- | --- | --- | --- |
| `DATABASE_URL` | 是 | 后端连接 PostgreSQL 的连接串。解析出 host / port / user / password / dbname，`?sslmode=require` 会被识别用于 TLS。 | Aiven 控制台 → 该 PostgreSQL 服务 → **Overview / Connection information** → 复制 **URI**。格式：`postgresql://USER:PASSWORD@HOST:PORT/DBNAME?sslmode=require` |
| `REDIS_URL` | 是 | 后端连接 Redis/Valkey 的连接串。`rediss://` 会自动开启 TLS；末尾 `/N` 表示使用第 N 号库。 | Aiven 控制台 → 该 Valkey 服务 → **Overview / Connection information** → 复制 **URI**。格式：`rediss://default:PASSWORD@HOST:PORT` |
| `AUTO_SETUP` | 是 | 设为 `true` 时，首次启动会根据上面的连接串生成 `config.yaml` 并创建初始管理员。这正是无持久盘场景需要的自动初始化。 | 固定填 `"true"` |
| `TZ` | 否 | 日志与定时任务使用的时区。 | 例如 `UTC`、`Asia/Shanghai`（默认 `Asia/Shanghai`） |

`render.yaml` 中 `DATABASE_URL` 与 `REDIS_URL` 标记为 `sync: false`，即**不落盘到仓库**：
Render 会在 Dashboard 里弹出输入框让你填写，值只存在于 Render 的服务环境中。

> 可选：若你想指定初始管理员邮箱，可在 Render Dashboard 追加 `ADMIN_EMAIL`（不写入 `render.yaml`，保持变量数最少）。

### 关于外部连接的 TLS

- Aiven PostgreSQL 使用非 5432 端口（如 `10317`），连接串里的 `sslmode=require` 已足够，无需额外 CA 文件。
- Aiven Valkey 必须用 `rediss://`（注意是两个 s），否则握手会失败。

---

## 2. 部署步骤（Render Blueprint）

1. **推送代码**：确保本次改动（`render.yaml`、`deploy/RENDER.md`、后端 URL 解析改动）已在你要部署的分支上。
2. **登录 Render** → 右上角 **New +** → **Blueprint**。
3. **选择仓库**：连接 GitHub/GitLab 并选中本项目仓库；Render 会自动发现根目录的 `render.yaml`。
4. **填写密钥变量**：蓝图预览页会列出 `DATABASE_URL`、`REDIS_URL`（`sync: false`），把 Aiven 的两个连接串粘贴进去。
   - 区域保持默认的 **Oregon（美国）**；实例类型保持 **Free**。
5. **Apply / Create**：Render 开始构建 Docker 镜像（多阶段：前端 pnpm 构建 → Go 构建 → 精简运行时）。
   免费实例首次构建通常需要数分钟。
6. **等待健康检查通过**：服务状态变为 **Live** 即表示 `healthCheckPath: /health` 返回成功。

### 验证是否部署成功

```bash
# 1) 健康检查（替换成你的 Render 服务地址）
curl -f https://<your-service>.onrender.com/health

# 2) 打开前端首页（若返回登录页 HTML 即为正常）
curl -sI https://<your-service>.onrender.com/ | head -1
```

查看启动日志（Render Dashboard → 你的服务 → **Logs**），应能看到：
数据目录初始化、`AUTO_SETUP` 自动配置、数据库迁移完成、HTTP 服务开始监听 **`$PORT`**。

---

## 3. 免费层注意事项

- **无持久磁盘**：容器重建后 `/app/data` 清空，`config.yaml` 会由环境变量自动重建。请**不要**依赖容器内文件做持久化。
- **休眠 / 冷启动**：免费实例在一段时间无流量后会休眠，下一次请求会触发冷启动（首个请求明显变慢，可能数十秒）。
- **额度限制**：免费实例每月有运行时长/流量配额；超出后服务可能被暂停直到下个周期。
- **单实例**：免费方案不支持水平扩展与零停机部署，重新部署期间会短暂中断。
- **构建缓存**：免费实例无持久构建缓存，重建可能较慢。

---

## 4. 与本机 docker-compose 的差异

本机 `deploy/docker-compose.yml` 使用**分项变量**（`DATABASE_HOST`、`REDIS_HOST` 等）指向同网络的
`postgres` / `redis` 容器，并可通过卷持久化 `/app/data`。两种方式**互不冲突且行为不变**：

- 显式设置了 `DATABASE_HOST` / `REDIS_HOST` 时，连接串**不会**覆盖它们（分项变量优先）。
- 未设置分项变量时，才使用 `DATABASE_URL` / `REDIS_URL`。
- 端口方面：显式 `SERVER_PORT` 优先；未设置时才回退到平台注入的 `PORT`，最后才用默认 `8080`。因此
  docker-compose（设置 `SERVER_PORT`）行为完全不变。

---

## 5. 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 健康检查一直失败 | 检查 `DATABASE_URL` 是否含 `sslmode=require`、`REDIS_URL` 是否为 `rediss://`；查看 Logs 里的连接报错。 |
| 启动报 Redis 握手错误 | `REDIS_URL` 写成了 `redis://`，Aiven 必须 `rediss://`。 |
| 重新部署后管理员又出现/或报管理员已存在 | 已按幂等处理：数据库已有管理员时会跳过创建，不会导致启动失败。 |
| 首页 502 | 冷启动中，稍等后重试；或构建失败，检查构建日志。 |
| 端口报错 | 无需手动设置 `PORT`，Render 自动注入，应用会读取。 |
