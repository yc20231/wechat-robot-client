# business-gateway

微信业务消息的确定性前置网关。它在 hp0912 内置 AI 之前完成群身份、管理员权限和业务数据隔离，业务失败时不会回退到 AI。

## 已实现

- `POST /internal/business/route`：供 `BusinessRouterPlugin` 同步调用。
- `GET|POST|PUT|DELETE /admin/groups`：使用 `X-Admin-Token` 管理群绑定。
- `POST /webhook/wechat`：使用 `X-Bot-Webhook-Token` 接收审计消息并去重。
- `help`、`inventory`、`status` 模块。
- 客户群固定 `customer_code`，管理员群同时校验群类型和全局管理员身份。
- 固定所有者、动态根管理员和动态普通管理员三级权限。
- 群内添加/移除管理员，绑定/改绑/解绑客户群和管理员群。
- 移除/降级管理员、改绑或解绑群需要同一操作者在 5 分钟内二次确认；新增管理员和首次绑定立即生效。
- 动态管理员保存到 `/data/admins.json`，管理变更追加到 `/data/audit.jsonl`。
- `Appid + NewMsgId` Webhook 去重和内部业务消息去重。

## 启动

```bash
cp .env.example .env
docker compose up -d --build
curl http://127.0.0.1:18080/healthz
```

群绑定、动态管理员和审计日志保存在 Docker 命名卷 `business-gateway-data`。`.env` 中的 `BACKEND_URL` 必须能从容器访问 ThinkPHP，`BOT_TOKEN` 必须与 ThinkPHP 的 `BOT_API_TOKEN` 一致。所有示例 token 上线前都必须替换为独立随机值。

必须显式配置不可移除的固定所有者：

```env
OWNER_WXIDS=wxid_4s48yvri1r7f22
CONFIRMATION_TTL_SEC=300
```

`OWNER_WXIDS` 是逗号分隔的 wxid，不是微信昵称。兼容旧部署：未设置 `OWNER_WXIDS` 时会把原 `ADMIN_WXIDS` 当作固定所有者；完成升级后应改用 `OWNER_WXIDS`。

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

## 权限

| 角色 | 范围 | 权限 |
|---|---|---|
| 固定所有者 | 全局、不可移除 | 全部权限；管理管理员群 |
| 动态根管理员 | 全局、持久化 | 添加/移除动态根管理员和普通管理员；管理客户群绑定 |
| 动态普通管理员 | 全局、持久化 | 在管理员群执行跨客户业务查询 |

移除动态根管理员会将其降为动态普通管理员；再次执行“移除管理员”才会删除全部管理员权限。管理员身份一经变更，对所有群立即生效。

管理员指令的目标必须是真实微信 `@` 成员。客户端从消息协议的 `atuserlist` 读取 wxid，不按昵称文本猜测身份。

## 命令

菜单和帮助在未绑定群也可使用：

```text
@机器人 菜单
@机器人 帮助
@机器人 业务帮助
```

`菜单`返回当前群可立即执行的精简功能列表；`帮助`按发送者身份返回跨群类型的完整命令、适用条件、示例和确认规则。旧的`业务帮助`与`帮助`等价。

菜单和帮助同时列出内置 AI 的图片能力：文字生成图片、引用用户图片进行修改、引用机器人生成图继续修改，以及“先发送参考图、再引用目标图”的双图替换。双图参考图必须来自同一会话、同一发送者且在 5 分钟内发送；图片请求由内置 AI 和 `text-to-image` Skill 执行，不进入库存业务模块。

客户群：

```text
查库存
查库存 红
库存查询 4056
```

全局管理员：

```text
查库存 270 红
查 270 库存 红
业务状态
```

客户群消息中的数字只作为库存关键词，永远不能覆盖该群绑定的 `customer_code`。

### 管理员角色

以下命令可在任意群发送，操作者必须是固定所有者或动态根管理员：

```text
@机器人 管理员列表
@机器人 添加 @W 根管理员
@机器人 移除 @W 根管理员
@机器人 确认移除 @W 根管理员
@机器人 添加 @W 管理员
@机器人 移除 @W 管理员
@机器人 确认移除 @W 管理员
```

添加管理员和根管理员立即生效；移除或降级必须由同一账号在同一个群内二次确认，默认 5 分钟失效。固定所有者不能被修改、移除或降级。

### 客户群绑定

固定所有者或动态根管理员在目标群内发送：

```text
@机器人 查看群绑定
@机器人 绑定客户 270
@机器人 改绑客户 365
@机器人 确认改绑 365
@机器人 解绑客户
@机器人 确认解绑客户 365
```

首次绑定立即生效，改绑需要二次确认。绑定前，网关会调用 ThinkPHP 的 `GET /api/bot/customers/resolve` 校验客户代号。空库存不代表客户不存在；禁用客户不能绑定。

### 管理员群绑定

只有固定所有者可以在目标群内发送：

```text
@机器人 绑定管理员群
@机器人 改绑管理员群
@机器人 确认改绑管理员群
@机器人 解绑管理员群
@机器人 确认解绑管理员群
```

首次绑定管理员群立即生效；`改绑管理员群` 用于把已绑定客户群转换为管理员群并需要确认。管理员群不设置 `customer_code`。

## HTTP 管理群绑定

```bash
curl -X POST http://127.0.0.1:18080/admin/groups \
  -H 'Content-Type: application/json' \
  -H 'X-Admin-Token: <ADMIN_TOKEN>' \
  -d '{"group_id":"customer-270@chatroom","group_name":"270客户群","type":"customer","customer_code":"270","enabled":true}'
```

管理员群不能设置 `customer_code`，也不能使用 `*` 作为客户编码。

## 飞牛离线构建

飞牛的 Docker Hub 代理无法拉取 `golang`/`alpine` 时，使用仓库私有 Go 工具链编译，再用阿里云运行时镜像封装：

```bash
cd /vol1/1000/wechat-robot/jiqiren/business-gateway

GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  ../.tools/go/bin/go build \
  -trimpath -ldflags='-s -w' \
  -o business-gateway-custom ./cmd/business-gateway

chmod 755 business-gateway-custom
docker build -f Dockerfile.fnos-runtime \
  -t jiqiren/business-gateway:local .
docker compose up -d --no-build
```

不要删除 `business-gateway-data` 卷，否则 `groups.json`、`admins.json` 和 `audit.jsonl` 会一起丢失。
