# Feed Video backend

Go 1.26.1。HTTP API 与 RabbitMQ Worker 分别运行。开发环境需要 Docker Compose。

## 本地启动（PowerShell）

在项目根目录执行：

```powershell
Copy-Item .env.example backend/.env
docker compose --env-file backend/.env up -d --wait
Set-Location backend
go run ./cmd
```

另开终端，进入 backend：

```powershell
go run ./cmd/worker
```

- API：http://localhost:8080
- 进程检查：GET /healthz
- MySQL 就绪检查：GET /readyz
- RabbitMQ 管理界面：http://localhost:15673，账号密码见本地 backend/.env
- Compose 仅将依赖服务绑定到本机回环地址，使用 13306 / 16379 / 15672 端口，避免覆盖本机默认端口服务。
- 停止依赖服务：在根目录执行 docker compose --env-file backend/.env down。该命令保留 MySQL 数据卷。
- 修改 MYSQL_DATABASE 或数据库初始化密码后，已有数据卷不会自动重新初始化；需要使用原来的凭据或迁移已有数据库。

.env.example 是本地开发示例，部署时应设置独立凭据和固定 JWT_SECRET。.env 不提交到 Git。

## 配置

默认读取 backend/configs/config.yaml；优先级为代码默认值 < YAML < 环境变量。
CONFIG_PATH 可以指定配置文件。不存在的显式路径、错误 YAML、未知字段或非法端口会阻止启动。
省略的开发默认配置文件可以使用默认值启动，但依然需要可用 MySQL。

MySQL 是必需依赖。Redis 或 RabbitMQ 不可用时，API 使用已有降级路径。
REDIS_ENABLED=false / RABBITMQ_ENABLED=false 可以显式关闭可选依赖。
分片上传需要 Redis；实时通知和异步任务需要 RabbitMQ。Worker 必须启用 RabbitMQ。
PPROF_ENABLED 默认关闭，现有 YAML 或环境变量可以启用它。

API 启动会使用 GORM AutoMigrate 补齐表结构；本次新增 refresh_expires_at 和 refresh_token 索引。
升级前已有的刷新令牌没有数据库有效期，需要重新登录。正式部署应审核迁移并备份数据库。

## 账号接口约定

请求使用 JSON；受保护接口发送 Authorization: Bearer <token>。

- POST /account/register：username、password。用户名 3～32 个字符，密码 8～72 字节。
- POST /account/login：username、password；返回 token、refresh_token、account_id、username。
- POST /account/refresh：refresh_token；返回新的访问令牌，原刷新令牌有效期不会延长。
- POST /account/logout：需要访问令牌；撤销该账号当前会话。
- POST /account/changePassword：需要访问令牌，提交 old_password、new_password；成功后重新登录。客户端提交的 username 不决定修改对象。
- POST /account/rename：需要访问令牌，提交 new_username；使用响应中的新 token 替换旧 token。
- 通知接口也检查数据库中的当前会话。原生 EventSource 可使用 /notification/stream?token=<token>；查询参数令牌不写入请求日志。

每个账号只有一个当前会话。访问令牌有效期 24 小时，刷新令牌有效期 7 天并持久化到 MySQL。
重新登录、刷新访问令牌、改名会替换旧访问令牌；登出和改密码会撤销访问及刷新令牌。
鉴权以 MySQL 记录为准，Redis 中的旧会话缓存不能授权请求。
缺少或错误凭据返回 401，非法输入返回 400，重复用户名返回 409。

视频发布 POST /video/publish 接收 title、description、play_url、cover_url。

## 自动检查

在根目录执行：

```powershell
.\scripts\check.ps1
```

等价的 backend 命令：

```text
gofmt -l cmd internal tests
go vet ./...
go test ./...
go build ./...
```

启动上面的 Compose 依赖后，可以运行：

```powershell
.\scripts\check.ps1 -Integration
```

集成测试默认连接本项目开发端口，创建随机命名的 feedvid_test_* 数据库并在完成后删除该测试库。
Redis 使用测试专属键前缀。使用专用测试环境时可覆盖 INTEGRATION_MYSQL_DSN、
INTEGRATION_REDIS_ADDR、INTEGRATION_AMQP_URL。数据库测试账号需要创建和删除测试库的权限。
不要将这些变量指向生产环境。

集成测试覆盖真实 MySQL 与 Redis 下的注册、登录、上传、发布、Feed、点赞/取消点赞、
刷新、登出、改密码、改名、刷新过期及旧缓存场景，并验证 Redis 不可用时的鉴权。
HTTP 流程使用 MQ 不可用的降级模式；另一个测试通过真实 RabbitMQ 检查热度事件 JSON 收发。
这不代表已有异步 Worker 的重试、去重、点赞计数一致性已完成验收。

GitHub Actions 分别运行单元检查和带真实依赖服务的集成测试。

## 点赞与事件一致性

点赞和取消点赞现在是幂等操作：重复请求返回成功，只有实际状态变化才修改计数。
点赞记录、视频点赞数/热度以及 Outbox 事件在一个 MySQL 事务中提交。
API 中的 Outbox 任务使用固定事件 ID、消息持久化、mandatory 返回检查及 Publisher Confirm。
投递失败会保留记录并按退避时间重试；确认丢失造成的重投由消费者去重处理。
点赞消费者不重复修改已经提交的计数，通知收件记录与通知写入同一事务。
缓存热度使用事件发生时间分桶，并在 Redis Lua 中完成去重与更新。
重试耗尽的点赞/通知消息进入死信队列，需排查后按原事件 ID 重放。

启动 API 时会添加 Outbox 扩展列、eventstore 收件记录表以及最新视频复合索引。
旧的 pending 视频发布记录可以继续投递，兼容迁移前没有事件 ID/payload 的记录。
正式部署应先审核迁移，按一次部署同时升级 API 和 Worker，避免新旧消费者混跑。

若旧版本已经造成点赞数与 likes 表不一致，在 backend 中运行：
go run ./cmd/reconcile-likes
该命令使用当前配置，逐个锁定视频并根据 likes 表修正 likes_count。
它不会生成点赞事件或通知，也不会重新计算混合了评论等因素的 popularity。

## Feed 游标

/feed/listLatest 和 /feed/listByFollowing 按 create_time DESC, id DESC 排序。
请求的 latest_time 使用毫秒，并将上一页 next_id_before 原样作为 id_before 一起传回。
响应包含 next_time、next_id_before、has_more；has_more 使用多查一条确定。
只传时间的旧请求仍可使用，但完整保留相同时间记录需要传复合游标。
关注列表兼容旧客户端在不传 ID 时使用的秒时间戳。

最新和关注列表由 MySQL 的索引查询确定 ID 顺序，实体继续使用本地/Redis 缓存及批量回源。
点赞数批量读取当前数据库值，避免实体缓存使计数冻结。
此前的全局 Redis 时间线仍由事件消费者维护，当前最新列表不依赖该投影是否已追上。

新增回归测试包含并发重复点赞、事务失败回滚、旧计数对账、Outbox 重试身份、
真实 broker 重投、不可路由检测、连接重建、旧表迁移、缓存锁释放和相同时间分页。