# 黑马点评 Go 后端

Go + Gin 版后端沿用 Java 版的主要接口路径和 `success`、`errorMsg`、`data` 响应结构。

## 代码结构

根目录的 `main.go` 只负责启动。`internal/app` 组装配置、数据库连接、Gin 路由和后台任务。业务代码按 `internal/user`、`internal/shop`、`internal/voucher`、`internal/social`、`internal/upload` 分包，HTTP 文件处理协议与响应，Service 处理业务流程，Repository 和 Storage 负责读写。`internal/httpapi` 提供统一的响应和分页参数处理，`internal/localtime` 负责兼容 Java 的日期时间格式。

## 本地运行

1. 安装 Go 1.26.4，配置已有的 MySQL 和 Redis 服务。已有 Java 版数据库可直接复用。只在创建空测试库时使用同级 Java 仓库的 `hmdp.sql`，原始 SQL 包含 `DROP TABLE`。
2. 从本地安全配置向进程传入下表的环境变量。已有前端可将原后端代理地址改为 Go 服务的监听地址。
3. 在 Go 仓库目录运行 `go run .`，用 `GET /ping` 检查服务，再请求 `GET /shop-type/list` 和 `GET /voucher/list/1`。

| 环境变量 | 用途 |
| --- | --- |
| `DIANPING_HTTP_ADDR` | HTTP 监听地址，默认 `:8080`，本机联调可设为 `127.0.0.1:8080` |
| `DIANPING_MYSQL_DSN` | 必填，填写已有 MySQL 服务的连接配置 |
| `DIANPING_REDIS_URL` | Redis 连接配置，例如本机无密码实例可用 `redis://127.0.0.1:6379/0`。会话、签到和异步秒杀需要 Redis |
| `DIANPING_SMS_ENDPOINT` | HTTPS 短信网关，接收 JSON `{"phone":"…","code":"…"}`，用 2xx 确认接收。短信网关配置好后才能通过接口发起验证码登录 |
| `DIANPING_UPLOAD_DIR` | 已存在、服务可读写的图片根目录。上传返回 `/blogs/...`，从 `/imgs/blogs/...` 读取 |

## 数据与行为

- Redis 会话沿用原版键名和 30 分钟续期规则，受保护路由接受原版 `authorization` Token。上传和删除图片要求登录，只有上传者可删除。
- 秒杀 Lua 原子检查库存与一人一单并投递至 `stream.orders`。消费者用原版 `g1` 消费组，MySQL 落单后才确认，监控该组的待处理消息与积压。切换前请停止旧 Java 消费者，Go 消费者会接管闲置的未确认消息。
- 分类和店铺详情的缓存使用 Go 版命名空间。附近商铺使用原版 `shop:geo:<分类ID>` 地理索引。点赞、共同关注和关注动态直接复用原版 Redis 键。新 Redis 实例需按原版预加载 GEO 索引。旧图片可从 `/imgs/blogs/...` 读取，旧文件没有 Go 版归属记录，需由存储管理员处理其删除。
- 本地回归验证运行 `go test -race ./...` 和 `go vet ./...`。测试使用模拟 MySQL 与本机临时 Redis 模拟器，需要允许绑定回环端口。

真实服务联调使用 `go test -run TestIntegrationLoginAndAsyncOrder -count=1 ./internal/app`。运行前配置 `DIANPING_TEST_MYSQL_DSN` 指向 `hmdp_test_` 开头的可丢弃数据库，并配置 `DIANPING_TEST_REDIS_URL` 指向专用测试 Redis。测试库只需导入原版 SQL。测试向库中插入用户、优惠券和订单，并通过本机 HTTPS 短信替身获取验证码，运行后可以重建测试库。
