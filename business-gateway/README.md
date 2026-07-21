# business-gateway

微信业务消息的确定性前置网关。它在 hp0912 内置 AI 之前完成群身份、管理员权限和业务数据隔离，业务失败时不会回退到 AI。

## 已实现

- `POST /internal/business/route`：供 `BusinessRouterPlugin` 同步调用。
- `GET|POST|PUT|DELETE /admin/groups`：使用 `X-Admin-Token` 管理群绑定。
- `POST /webhook/wechat`：使用 `X-Bot-Webhook-Token` 接收审计消息并去重。
- `help`、`inventory`、`status` 模块。
- 客户群固定 `customer_code`，管理员群同时校验群类型和 `ADMIN_WXIDS`。
- `Appid + NewMsgId` Webhook 去重和内部业务消息去重。

## 启动

```bash
cp .env.example .env
docker compose up -d --build
curl http://127.0.0.1:18080/healthz
```

群绑定保存在 Docker 命名卷 `business-gateway-data`；首次启动后通过下方管理接口添加。`.env` 中的 `BACKEND_URL` 必须能从容器访问 ThinkPHP，`BOT_TOKEN` 必须与 ThinkPHP 的 `BOT_API_TOKEN` 一致。所有示例 token 上线前都必须替换为独立随机值。

## 客户端配置

上游管理后台动态创建客户端时使用固定环境变量列表，因此飞牛部署使用机器人已有的 Skills 挂载目录。把 `client-config.example.json` 复制为宿主机上的：

```text
<.deploy/local绝对路径>/wechat-robot/<robot_code>/data/skills/.business-gateway.json
```

文件权限设为 `600`，其中 `token` 与网关的 `INTERNAL_ROUTE_TOKEN` 相同。客户端容器内会自动从 `/data/skills/.business-gateway.json` 读取。

不经过上游管理后台、直接运行客户端时，也可以使用环境变量：

```env
BUSINESS_GATEWAY_URL=http://business-gateway:8080
BUSINESS_GATEWAY_TOKEN=<与 INTERNAL_ROUTE_TOKEN 相同>
BUSINESS_GATEWAY_TIMEOUT_SEC=5
```

URL 留空时插件保持禁用，不影响现有 AI。URL 已配置但网关不可用时，插件会停止后续 AI 并返回固定错误，这是业务数据的故障闭合行为。

## 命令

客户群：

```text
业务帮助
查库存
查库存 红
库存查询 4056
```

管理员白名单成员：

```text
查库存 270 红
查 270 库存 红
业务状态
```

客户群消息中的数字只作为库存关键词，永远不能覆盖该群绑定的 `customer_code`。

## 管理群绑定

```bash
curl -X POST http://127.0.0.1:18080/admin/groups \
  -H 'Content-Type: application/json' \
  -H 'X-Admin-Token: <ADMIN_TOKEN>' \
  -d '{"group_id":"customer-270@chatroom","group_name":"270客户群","type":"customer","customer_code":"270","enabled":true}'
```

管理员群不能设置 `customer_code`，也不能使用 `*` 作为客户编码。
