# 膳计 ShanJi

家庭膳食规划助手——替你决定"这周吃什么"：按全家饮食约束自动生成一周三餐菜单、买菜清单和菲佣看得懂的双语做菜指引。

- PRD: [docs/PRD.md](docs/PRD.md)
- 技术方案: [docs/TECH_DESIGN.md](docs/TECH_DESIGN.md)

## 架构

API-first：业务逻辑全部在 Go 后端的 `internal/domain`（零 HTTP/SQL 依赖的纯领域包），Web 前端只是第一个客户端，未来 iOS 消费同一套 `/api/v1`。

```
cmd/server/        入口（自动迁移 + 优雅停机）
internal/
  domain/          纯领域层：planner 求解器 / rules / shopping（业务逻辑唯一住所）
  api/             薄 HTTP 适配层：路由 / 中间件 / DTO
  store/           pgx 存储层
  ai/              Claude 编排（意图解析 / 食谱生成 / 翻译）
  config/
migrations/        golang-migrate SQL（嵌入二进制，启动自动执行）
web/               Next.js 前端（待建）
```

## 本地开发

前置：Go 1.26+，Docker Desktop。

```bash
make dev        # 起 PostgreSQL(5433) + API 服务(8090)，自动迁移
```

验证：

```bash
curl http://localhost:8090/health
# {"success":true,"data":{"status":"ok"}}

TOKEN=$(curl -s -X POST http://localhost:8090/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

curl -s http://localhost:8090/api/v1/household -H "Authorization: Bearer $TOKEN"
```

默认账号：`admin` / `admin123`。

端口约定（避免与本机其他项目冲突）：PostgreSQL `5433`、API `8090`、Web dev `3002`。
