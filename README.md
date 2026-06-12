# Backpack Service

背包服务是一个 Go HTTP 服务，负责消费抽卡完成事件并保存：

- 用户背包库存
- 抽卡批次日志
- 每条出货记录

服务使用 Postgres 作为事实源，Kafka 作为抽卡事件入口。它不负责资产扣减，也不直接执行抽卡。

## 身份边界

HTTP 查询接口必须经过网关。服务只信任网关注入的请求头：

```http
X-Internal-Token: <网关内部调用密钥>
X-User-Id: <网关校验后的用户 UUID>
```

## 配置

必填环境变量：

```text
DATABASE_URL=postgres://...
INTERNAL_TOKEN=...
KAFKA_BROKERS=kafka:9092
```

可选环境变量：

```text
PORT=8080
MAX_PAGE_LIMIT=100
KAFKA_TOPIC=gacha.pull_completed.v1
KAFKA_GROUP_ID=backpack-service
CONSUMER_ENABLED=true
```

## Kafka

服务消费 `gacha.pull_completed.v1`，按 `event_id` 幂等写入数据库。数据库事务成功后才提交 Kafka offset。

## API

健康检查：

```http
GET /health
GET /ready
```

背包：

```http
GET /v1/me/inventory?limit=50&cursor=<next_cursor>
GET /v1/me/inventory/{item_id}
```

抽卡批次日志：

```http
GET /v1/me/pull-events?limit=50&cursor=<next_cursor>&banner_id=limited-character-001
GET /v1/me/pull-events/{event_id}
```

出货记录：

```http
GET /v1/me/pull-records?limit=50&cursor=<next_cursor>&banner_id=limited-character-001&rarity=5&item_type=character
```

## 本地开发

运行测试：

```bash
go test ./...
```

一键启动整链路：

```bash
docker compose up --build
```

这会启动 Postgres、数据库迁移、Redis、Redpanda、`gacha-engine-service` 和 `backpack-service`。

本地端口：

- `gacha-engine-service`: `127.0.0.1:8082`
- `backpack-service`: `127.0.0.1:8083`
- Postgres: `127.0.0.1:5433`
- Redis: `127.0.0.1:6380`
- Kafka: `127.0.0.1:19093`

示例抽卡后查询背包：

```bash
curl -X POST http://127.0.0.1:8082/v1/me/pulls \
  -H "Content-Type: application/json" \
  -H "X-Internal-Token: local-dev-token" \
  -H "X-User-Id: ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e" \
  -d "{\"banner_id\":\"limited-character-001\",\"count\":10,\"seed\":\"demo\"}"

curl http://127.0.0.1:8083/v1/me/inventory \
  -H "X-Internal-Token: local-dev-token" \
  -H "X-User-Id: ae6b9d2e-9bb0-42c7-950f-c38ab6d7195e"
```

网关接入示例：

```text
backpack=/api/v1/backpack|http://backpack-service.backpack-service.svc.cluster.local/v1
```

