# 飞牛 OS 部署：hp0912 微信机器人平台 + 自定义客户端

本文档记录当前已经实际验证的飞牛 OS 部署方式，以及库存业务网关的升级和回滚流程。

## 0. 当前状态

截至 2026-07-22：

| 能力 | 状态 |
|---|---|
| hp0912 管理后台、MySQL、Redis、Qdrant | 已部署 |
| 测试微信扫码登录、消息收发 | 已验证 |
| 客户端和协议服务端重启后恢复登录态 | 已验证 |
| hp0912 内置 AI | 已验证 |
| AI 最终文本回复尾注 | 已部署并验证 |
| `BusinessRouterPlugin` | 已部署，前置短路和非业务 AI 放行已验证 |
| `business-gateway` | 已部署，健康检查、Token 和基础路由已验证 |
| ThinkPHP 健康检查和库存接口 | 已连通验证 |
| 固定所有者、动态管理员和群内绑定 | 代码和测试已完成，待执行本次升级 |
| ThinkPHP 客户代号校验接口 | 代码已完成，待发布 |

当前飞牛已完成“微信收发 + 内置 AI + AI 回复尾注 + 前置业务路由”基础联调。本次升级增加固定所有者、动态管理员、群内客户/管理员群绑定、二次确认和持久化审计。升级验收完成前不得接入真实客户群，也不得让 AI 猜测库存、订单、价格、余额或排期。

## 1. 目标架构

```text
hp0912/wechat-robot-client
    -> BusinessRouterPlugin（前置业务路由）
       ├─ 业务消息 -> business-gateway -> ThinkPHP
       └─ 非业务消息 -> hp0912 内置 AI
    -> Webhook（审计、同步和异常补偿）
```

职责边界：

- hp0912 平台负责微信登录、收发消息、内置 AI 和机器人管理。
- 自定义客户端负责 AI 尾注和前置业务路由。
- `business-gateway` 负责群绑定、身份判断、权限、去重和固定业务指令。
- ThinkPHP 负责库存等业务数据，并再次执行 `customer_code` 数据隔离。

旧的 `wangzhan/bot-mcp` 不是新架构的生产网关。它在新网关完成前只保留作代码参考，不启动、不删除。

## 2. 使用范围与安全前提

当前项目按内部、非商业测试范围部署。仓库 README 和许可证包含用途限制；用途发生变化时必须重新核对许可证和授权要求。

部署要求：

- 飞牛 OS 已安装 Docker、Docker Compose、Git 和 OpenSSL。
- 使用专用测试微信号，不使用主微信号首次验证。
- 飞牛具有固定局域网 IP，且不把管理端口转发到公网。
- 所有默认密码、示例 Token 和 API Key 都必须替换。
- 管理后台挂载 `/var/run/docker.sock`，具有宿主机 Docker 管理权限，只能部署在可信主机和可信网络。

当前部署根目录：

```text
/vol1/1000/wechat-robot/jiqiren
```

## 3. 部署基础平台

### 3.1 停用旧服务

旧 `bot-mcp` 若仍在运行，先禁用自动重启并停止，但保留容器、镜像和目录：

```bash
docker update --restart=no bot-mcp
docker stop bot-mcp
```

如果 Cloudflare Tunnel 只为旧服务提供公网入口，也应停用；如果还承载其他服务，必须先在 Cloudflare 控制台拆分路由。任何已经公开显示过的 Tunnel Token 都必须轮换。

### 3.2 克隆项目

```bash
cd /vol1/1000/wechat-robot
git clone --depth 1 https://github.com/yc20231/wechat-robot-client.git jiqiren
```

飞牛访问 GitHub 出现 `GnuTLS recv error (-110)` 时使用 HTTP/1.1：

```bash
git -c http.version=HTTP/1.1 clone --depth 1 \
  https://github.com/yc20231/wechat-robot-client.git jiqiren
```

进入部署目录并创建外部网络：

```bash
cd /vol1/1000/wechat-robot/jiqiren/.deploy/local
docker network inspect wechat-robot >/dev/null 2>&1 || docker network create wechat-robot
```

飞牛共享目录可能把克隆文件显示为 `000` 权限。必要时修正：

```bash
chmod 755 . gen-self-signed-cert.sh reset.sh
chmod 644 docker-compose.yml docker-compose2.yml nginx.conf my.cnf redis.conf
chmod 700 secrets
chmod 600 secrets/*.txt
```

### 3.3 选择 Compose 文件

当前部署只使用：

```text
.deploy/local/docker-compose.yml
```

以下文件不用于当前实例：

```text
docker-compose.yml.original  原始配置备份
docker-compose2.yml          另一套 secrets 示例
```

执行普通 `docker compose` 命令时，Compose 默认读取当前目录中的 `docker-compose.yml`。

### 3.4 HTTPS 证书

在 `.deploy/local` 目录执行，IP 替换为飞牛实际局域网 IP：

```bash
./gen-self-signed-cert.sh --ip <FNOS_LAN_IP>
```

生成文件：

```text
secrets/nginx/tls.crt
secrets/nginx/tls.key
```

浏览器首次访问 `https://<FNOS_LAN_IP>:8443` 会提示自签名证书不受信任，局域网测试时确认继续访问即可。

### 3.5 密码和 Token

至少替换以下默认值，并保证同一密码在 Compose 中的所有引用完全一致：

- MySQL root 密码和业务用户密码
- Redis 密码
- Qdrant API Key
- `SESSION_SECRET`
- `LOGIN_TOKEN`
- `WECHAT_SERVER_TOKEN`
- `SLIDER_TOKEN`（需要滑块验证时）
- `OPENAI_API_KEY` 和 `THIRD_PARTY_API_KEY`（启用对应能力时）

随机值可用以下命令生成：

```bash
openssl rand -hex 32
```

Token 用途：

| 配置 | 用途 |
|---|---|
| `LOGIN_TOKEN` | 登录 hp0912 机器人管理后台 |
| `WECHAT_SERVER_TOKEN` | hp0912 后台访问 `wechat-server` 的内部 API Token |
| `SLIDER_TOKEN` | hp0912 后台访问滑块服务的授权凭证，并非每次扫码都会使用 |
| `SESSION_SECRET` | 管理后台会话签名密钥 |

`SLIDER_TOKEN` 与 WeChatPadPro 授权无关。仓库示例 Token 可能过期；只有登录触发滑块验证时才会成为阻塞项。

包含真实密码的文件必须限制权限：

```bash
chmod 600 docker-compose.yml .deployment-credentials.txt
```

不得提交 `.deployment-credentials.txt`、真实 Compose 密钥、证书私钥或 Token。

### 3.6 启动平台

```bash
cd /vol1/1000/wechat-robot/jiqiren/.deploy/local
docker compose config >/dev/null
docker compose pull
docker compose up -d
docker compose ps
```

主要入口：

| 地址 | 用途 |
|---|---|
| `https://<FNOS_LAN_IP>:8443` | hp0912 机器人管理后台 |
| `http://<FNOS_LAN_IP>:8090` | `wechat-server` 管理页面 |

主要端口：

```text
8443  管理后台 HTTPS
8080  HTTP 跳转 HTTPS
8090  wechat-server
6333  Qdrant REST
6334  Qdrant gRPC
9200  小红书 MCP
3000  网易云音乐
```

这些端口不得通过路由器直接暴露到公网。当前 Compose 会把部分辅助服务绑定到局域网地址，远程访问应使用 VPN，并进一步收紧防火墙。

### 3.7 配置 wechat-server

首次访问：

```text
http://<FNOS_LAN_IP>:8090
```

上游默认管理员通常为：

```text
用户名：root
密码：123456
```

登录后立即修改默认密码，然后在“设置 -> 个人设置”生成访问令牌。把生成值写入当前 `docker-compose.yml` 的：

```yaml
WECHAT_SERVER_TOKEN: "<真实访问令牌>"
```

重建后台服务：

```bash
docker compose config >/dev/null
docker compose up -d --force-recreate wechat-robot-admin-backend
```

网页提示“请求次数过多”时停止重试，等待限流恢复；提示“未登录或 token 无效”时清理浏览器对该地址的站点数据并重新登录。不要为处理网页登录问题删除 `wechat-server/data`。

### 3.8 登录机器人并验收基础链路

1. 使用 `LOGIN_TOKEN` 登录 `https://<FNOS_LAN_IP>:8443`。
2. 使用现有机器人实例或创建一个测试实例。
3. 使用专用测试微信扫码登录。
4. 在管理后台配置内置 AI 的模型、API Key、系统提示词、知识库和记忆。
5. 使用测试群 `@机器人` 验证消息收发和 AI 回复。

业务路由尚未完成部署联调时，系统提示词必须明确：不得查询、猜测或编造库存、订单、价格、余额、排期和客户资料；遇到此类问题只能返回固定的“业务查询功能正在配置中”提示。提示词只是临时保护，不能替代前置业务路由和后端权限校验。

管理后台动态创建两个容器：

```text
server_<robot_code>  闭源微信协议服务端
client_<robot_code>  开源消息处理和 AI 客户端
```

分别重启并验证登录态恢复：

```bash
docker restart client_<robot_code>
docker restart server_<robot_code>
```

每次只重启一个容器，等待恢复在线后再测试另一个。不要因为短暂离线重新扫码。

## 4. 自定义客户端：AI 回复尾注

### 4.1 行为

当前修改位于：

```text
plugin/plugins/ai_chat.go
```

仅对最终 AI 纯文本回复追加：

```text
本消息由AI生成回复
```

工具结果、业务结果、图片、语音和错误提示不追加。配置项：

```env
AI_REPLY_FOOTER_ENABLED=true
AI_REPLY_FOOTER=本消息由AI生成回复
```

未传环境变量时默认启用上述尾注。

### 4.2 飞牛本地编译

当前飞牛的 Docker Hub 代理可能对 `docker/dockerfile` 和 `golang` 返回 `401`，IPv6 镜像下载也可能中断。已验证的稳定方式是在飞牛上安装项目私有 Go 工具链并直接编译，不修改系统 Go。

当前仓库使用 Go 1.25，已验证 Go 1.25.8 Linux AMD64：

```bash
cd /vol1/1000/wechat-robot/jiqiren
mkdir -p .tools

curl -4 -fL --retry 5 --retry-delay 3 \
  -o .tools/go1.25.8.linux-amd64.tar.gz \
  https://go.dev/dl/go1.25.8.linux-amd64.tar.gz

echo 'ceb5e041bbc3893846bd1614d76cb4681c91dadee579426cf21a63f2d7e03be6  .tools/go1.25.8.linux-amd64.tar.gz' \
  | sha256sum -c -

tar -C .tools -xzf .tools/go1.25.8.linux-amd64.tar.gz
export PATH="$PWD/.tools/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct
export GOMODCACHE="$PWD/.tools/gomodcache"
export GOCACHE="$PWD/.tools/gobuildcache"

GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build \
  -trimpath \
  -ldflags='-s -w -X main.Version=custom-footer-v1' \
  -o wechat-robot-client-custom

chmod 755 wechat-robot-client-custom
```

### 4.3 封装运行时镜像

根目录的 `Dockerfile.fnos-runtime` 使用现有 `silk-base` 运行时，并强制给二进制设置执行权限：

```dockerfile
COPY --chmod=0755 wechat-robot-client-custom ./wechat-robot-client
RUN test -x /app/wechat-robot-client
```

构建：

```bash
cd /vol1/1000/wechat-robot/jiqiren
mkdir -p .runtime-build
cp wechat-robot-client-custom .runtime-build/
cp Dockerfile.fnos-runtime .runtime-build/Dockerfile
cd .runtime-build

docker build -t jiqiren/wechat-robot-client:footer-v2 .

docker run --rm --entrypoint /bin/sh \
  jiqiren/wechat-robot-client:footer-v2 \
  -c 'ls -l /app/wechat-robot-client && test -x /app/wechat-robot-client && echo "executable OK"'
```

如果镜像内二进制不可执行，客户端启动会报：

```text
exec: "/app/wechat-robot-client": permission denied
```

必须修复镜像，不要重新扫码或删除服务端数据。

### 4.4 镜像标签和切换规则

当前管理后台后端把客户端镜像名硬编码为：

```text
registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest
```

因此切换前必须保留官方回滚标签：

```bash
docker tag \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:upstream-20260721

docker tag \
  jiqiren/wechat-robot-client:footer-v2 \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest
```

然后在管理后台只执行：

```text
删除客户端容器 -> 创建客户端容器
```

不要删除服务端容器，不要退出微信，不要重新扫码。管理后台看到本地 `latest` 已存在时不会再次拉取。

验证：

```bash
docker inspect client_<robot_code> \
  --format '镜像={{.Image}} 状态={{.State.Status}}'
```

测试群发送普通 AI 消息，回复末尾必须出现 AI 尾注。

### 4.5 回滚

自定义客户端失败时：

```bash
docker tag \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:upstream-20260721 \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest
```

删除创建失败的客户端容器，再通过管理后台创建客户端容器。服务端和 Redis 保存登录态，正常情况下不需要重新扫码。

出现以下错误表示客户端没有运行，而不是联系人真的不存在：

```text
lookup client_<robot_code> on 127.0.0.11:53: no such host
```

### 4.6 上游升级

禁止直接点击管理后台“更新镜像”。该操作会拉取官方镜像并覆盖自定义 `latest` 标签，导致 AI 尾注和后续业务路由消失。

正确升级顺序：

```text
拉取/合并上游客户端代码
  -> 保留并重新应用本项目修改
  -> 编译新的自定义二进制
  -> 构建新的固定版本标签
  -> 保留新的官方回滚标签
  -> 自定义镜像重新标记为 latest
  -> 只重建客户端容器
  -> 验证 AI 尾注和业务路由
```

多个标签指向同一镜像 ID时不会重复占用完整磁盘空间。查看标签：

```bash
docker image ls --no-trunc jiqiren/wechat-robot-client
docker image ls --no-trunc registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client
```

## 5. 前置业务路由

`BusinessRouterPlugin` 已注册在 hp0912 AI 插件之前，并同步调用：

```text
POST http://business-gateway:8080/internal/business/route
```

响应语义：

```json
{"handled": true, "reply": "客户 270 当前库存：..."}
```

非业务消息：

```json
{"handled": false}
```

业务错误也必须阻止 AI：

```json
{"handled": true, "error": "库存服务暂不可用"}
```

处理规则：

```text
handled=true  -> 插件发送确定性业务回复，停止后续 AI
handled=false -> 继续 hp0912 内置 AI
业务错误      -> 插件发送固定错误，停止后续 AI
```

`business-gateway` 目标目录：

```text
/Users/Admin/Documents/ios_app/jiqiren/business-gateway
```

该目录和接口已经实现并完成基础联调，包含群绑定、全局管理员、库存查询、管理 API、持久化审计和消息去重。部署说明见 `business-gateway/README.md`。

上游管理后台创建客户端容器时使用固定环境变量列表，不会透传新增变量。飞牛部署必须把客户端配置写入现有 Skills 挂载目录：

```text
.deploy/local/wechat-robot/<robot_code>/data/skills/.business-gateway.json
```

客户端容器会在 `/data/skills/.business-gateway.json` 读取网关 URL、内部 Token 和超时。该文件含密钥，权限必须设为 `600`，不得提交。

### 5.1 发布 ThinkPHP 客户校验接口

先发布网站仓库中的以下文件，再升级网关：

```text
backend/app/controller/BotCustomerController.php
backend/app/service/BotCustomerService.php
backend/route/app.php
```

在 ThinkPHP 后端部署目录执行：

```bash
php -l app/controller/BotCustomerController.php
php -l app/service/BotCustomerService.php
php -l route/app.php
php think clear
```

然后从飞牛测试。命令从 `.env` 读取 Token，不会把 Token 打印出来：

```bash
cd /vol1/1000/wechat-robot/jiqiren/business-gateway
set -a
. ./.env
set +a

curl -sS \
  --connect-timeout 10 \
  --max-time 20 \
  -H "X-Bot-Token: ${BOT_TOKEN}" \
  -H 'User-Agent: bot-mcp/business-gateway-1.0' \
  -w '\nHTTP_STATUS=%{http_code}\n' \
  "${BACKEND_URL%/}/api/bot/customers/resolve?customer_code=270"
```

预期 `HTTP_STATUS=200`，已启用客户返回 `"exists":true`，不存在或禁用客户返回 `"exists":false`。

### 5.2 更新源码和网关配置

飞牛仓库允许 `.deploy/local` 保留本机配置改动；升级前先确认本次上游变更不会覆盖它们：

```bash
cd /vol1/1000/wechat-robot/jiqiren
git fetch origin
git diff --name-only HEAD..origin/main
git merge --ff-only origin/main
git log -1 --oneline
```

编辑 `business-gateway/.env`，保留现有 Token 和 URL，并确保包含：

```env
BINDINGS_FILE=/data/groups.json
ADMINS_FILE=/data/admins.json
AUDIT_FILE=/data/audit.jsonl
OWNER_WXIDS=wxid_4s48yvri1r7f22
CONFIRMATION_TTL_SEC=300
```

`wxid_4s48yvri1r7f22` 是微信名“Y”的固定所有者 wxid。固定所有者来自环境变量，不写入动态管理员文件，不能被群内命令移除。旧 `ADMIN_WXIDS` 可以暂时保留，但 `OWNER_WXIDS` 存在时以它为准。

### 5.3 构建并切换网关

Docker Hub 代理可用时可直接执行 `docker compose up -d --build`。飞牛已出现过 `docker.fnnas.com ... 401 Unauthorized`，推荐使用本地 Go 工具链：

```bash
cd /vol1/1000/wechat-robot/jiqiren/business-gateway

../.tools/go/bin/go test ./...
GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  ../.tools/go/bin/go build \
  -trimpath -ldflags='-s -w' \
  -o business-gateway-custom ./cmd/business-gateway
chmod 755 business-gateway-custom

GATEWAY_CONTAINER="$(docker compose ps -q business-gateway)"
if [ -n "$GATEWAY_CONTAINER" ]; then
  GATEWAY_IMAGE_ID="$(docker inspect "$GATEWAY_CONTAINER" --format '{{.Image}}')"
  docker tag "$GATEWAY_IMAGE_ID" jiqiren/business-gateway:rollback-before-admin-v2
fi

docker build -f Dockerfile.fnos-runtime \
  -t jiqiren/business-gateway:local .
docker compose config -q
docker compose up -d --no-build --force-recreate
docker compose ps
curl -sS -w '\nHTTP_STATUS=%{http_code}\n' http://127.0.0.1:18080/healthz
```

预期网关为 `Up`，健康检查返回 `{"status":"ok"}` 和 HTTP 200。命名卷 `business-gateway-data` 必须保留。

### 5.4 构建并切换客户端

```bash
cd /vol1/1000/wechat-robot/jiqiren

.tools/go/bin/go test ./plugin/plugins \
  -run 'Test(BusinessRouter|LoadBusinessRouter|InvalidConfiguredBusinessRouter|AppendAIReplyFooter)'

GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  .tools/go/bin/go build \
  -trimpath \
  -ldflags='-s -w -X main.Version=business-admin-v2' \
  -o wechat-robot-client-custom
chmod 755 wechat-robot-client-custom

mkdir -p .runtime-build
cp wechat-robot-client-custom .runtime-build/
cp Dockerfile.fnos-runtime .runtime-build/Dockerfile
docker build -t jiqiren/wechat-robot-client:business-admin-v2 .runtime-build

CLIENT='client_xiW55bPyM3D4o6s6'
UPSTREAM='registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client'
CURRENT_IMAGE_ID="$(docker inspect "$CLIENT" --format '{{.Image}}')"
docker tag "$CURRENT_IMAGE_ID" "$UPSTREAM:rollback-before-admin-v2"
docker tag jiqiren/wechat-robot-client:business-admin-v2 "$UPSTREAM:latest"
```

随后在 hp0912 管理后台只删除并重新创建客户端容器 `client_xiW55bPyM3D4o6s6`。不要删除或重建 `server_xiW55bPyM3D4o6s6`、MySQL、Redis，不要退出微信或重新扫码。

验证运行镜像和配置：

```bash
docker inspect client_xiW55bPyM3D4o6s6 \
  --format '镜像={{.Image}} 状态={{.State.Status}}'
docker exec client_xiW55bPyM3D4o6s6 \
  /bin/sh -lc 'test -r /data/skills/.business-gateway.json && stat -c "配置权限=%a 配置大小=%s" /data/skills/.business-gateway.json'
docker logs --since 5m client_xiW55bPyM3D4o6s6 2>&1 \
  | grep -E '启动 版本|BusinessRouter|初始化失败|panic' \
  | tail -30
```

版本日志应包含 `business-admin-v2`，配置权限应为 `600`。

### 5.5 微信验收

先只在测试群验证。由固定所有者 Y 发送：

```text
@机器人 管理员列表
@机器人 查看群绑定
```

选择一个测试账号 W 验证普通管理员的全局增删：

```text
@机器人 添加 @W 管理员
@机器人 确认添加 @W 管理员
@机器人 移除 @W 管理员
@机器人 确认移除 @W 管理员
```

再验证客户群完整生命周期，`270` 替换为 ThinkPHP 已启用的测试客户代号：

```text
@机器人 绑定客户 270
@机器人 确认绑定 270
@机器人 查库存
@机器人 解绑客户
@机器人 确认解绑客户 270
```

普通消息应继续进入内置 AI，实时业务失败应被网关拦截且不能进入 AI。检查持久化文件和审计日志：

```bash
cd /vol1/1000/wechat-robot/jiqiren/business-gateway
docker compose exec business-gateway /bin/sh -lc '
  ls -l /data/groups.json /data/admins.json /data/audit.jsonl
  tail -n 20 /data/audit.jsonl
'
```

重启网关后再次发送“管理员列表”和“查看群绑定”，确认动态管理员和群绑定仍然存在：

```bash
docker compose restart business-gateway
docker compose ps
```

### 5.6 本次升级回滚

客户端回滚：

```bash
docker tag \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:rollback-before-admin-v2 \
  registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest
```

然后仍然只在管理后台删除并重新创建客户端容器。网关回滚：

```bash
cd /vol1/1000/wechat-robot/jiqiren/business-gateway
docker tag \
  jiqiren/business-gateway:rollback-before-admin-v2 \
  jiqiren/business-gateway:local
docker compose up -d --no-build --force-recreate
```

回滚镜像时不要删除 `business-gateway-data` 卷。新版创建的 `/data/admins.json` 和 `/data/audit.jsonl` 可以保留，旧版不会使用它们。

## 6. 权限模型

```text
客户群：group_id 固定绑定一个 customer_code，只能查询该客户

管理员群：群类型必须为 admin，且发送者必须是全局管理员
```

角色权限：

| 角色 | 作用域 | 权限 |
|---|---|---|
| 固定所有者 Y | 全局、不可移除 | 全部权限和管理员群管理 |
| 动态根管理员 | 全局 | 管理动态管理员；绑定/改绑/解绑客户群 |
| 动态普通管理员 | 全局 | 在管理员群跨客户查询 |

动态根管理员可以添加或移除其他动态根管理员和普通管理员。移除动态根管理员会先降为普通管理员；固定所有者不能被移除或降级。

所有变更指令都需要同一操作者在同一个群内于 5 分钟内确认。管理员目标取自微信消息真实 `atuserlist`，不能使用昵称文本冒充。

角色管理命令：

```text
@机器人 管理员列表
@机器人 添加 @W 根管理员
@机器人 确认添加 @W 根管理员
@机器人 移除 @W 根管理员
@机器人 确认移除 @W 根管理员
@机器人 添加 @W 管理员
@机器人 确认添加 @W 管理员
@机器人 移除 @W 管理员
@机器人 确认移除 @W 管理员
```

客户群绑定命令：

```text
@机器人 查看群绑定
@机器人 绑定客户 270
@机器人 确认绑定 270
@机器人 改绑客户 365
@机器人 确认改绑 365
@机器人 解绑客户
@机器人 确认解绑客户 365
```

管理员群命令仅固定所有者可用：

```text
@机器人 绑定管理员群
@机器人 确认绑定管理员群
@机器人 改绑管理员群
@机器人 确认改绑管理员群
@机器人 解绑管理员群
@机器人 确认解绑管理员群
```

管理员群不使用 `*` 作为客户编码。群绑定和动态管理员保存在网关数据卷，变更审计追加到 `/data/audit.jsonl`。

## 7. Webhook

Webhook 用于审计、联系人同步和异常补偿，不作为业务消息的第一处理入口：

```text
http://business-gateway:8080/webhook/wechat
```

自定义请求头：

```text
X-Bot-Webhook-Token: <随机长字符串>
```

hp0912 会发送包含 `AddMsgs` 的批次，并在 URL 后追加 `robot_id`、`robot_code`、`robot_wxid`。去重键：

```text
Appid + ":" + NewMsgId
```

## 8. 验收清单

基础平台：

- [x] 管理后台可访问
- [x] 测试微信扫码登录
- [x] 群消息和 AI 回复正常
- [x] 客户端重启后自动恢复
- [x] 协议服务端重启后自动恢复
- [x] 自定义客户端运行
- [x] AI 文本回复带尾注
- [ ] 连续运行 48-72 小时无异常

业务代码：

- [x] `BusinessRouterPlugin` 注册在 AI 之前并支持短路
- [x] `business-gateway` 同步路由和 Token 校验
- [x] 客户群、管理员群和实验模块权限测试
- [x] 后端失败、非业务放行和重复消息测试
- [x] Webhook Token、去重、机器人自身消息过滤和审计入口测试
- [x] 新增测试、竞态检测、`go vet` 和完整构建

飞牛部署验收：

- [x] 自定义客户端基础业务镜像运行
- [x] `business-gateway` 基础同步路由可用
- [ ] 客户群只能查询绑定客户
- [ ] 管理员群普通成员不能跨客户查询
- [x] 管理员权限拒绝已通过内部路由验证
- [x] 业务失败不会落入 AI
- [x] 非业务消息继续进入内置 AI
- [ ] 固定所有者 Y 不可移除
- [ ] 动态根管理员和普通管理员全局增删
- [ ] 客户群和管理员群绑定、改绑、解绑
- [ ] 网关重启后管理员和群绑定仍存在
- [ ] Webhook 去重和审计可用

业务链路全部完成前，不绑定真实客户群。

## 9. 故障排查

| 现象 | 处理 |
|---|---|
| GitHub 克隆出现 `GnuTLS recv error (-110)` | 使用 `git -c http.version=HTTP/1.1 clone --depth 1` |
| Docker Hub 前端或 `golang` 返回 `401` | 使用项目私有 Go 工具链直接编译，再用 `Dockerfile.fnos-runtime` 封装 |
| Docker 拉取层时 IPv6 `connection reset` | 使用 `curl -4` 下载官方 Go 工具链，不关闭 NAS 全局 IPv6 |
| 客户端报 `permission denied` | 使用 `COPY --chmod=0755` 重建镜像 |
| 后台报 `lookup client_... no such host` | 客户端容器不存在或启动失败，先查 `docker ps -a` 和客户端日志 |
| 自定义尾注突然消失 | 检查是否点击“更新镜像”，核对运行镜像 ID和 `latest` 标签 |
| wechat-server 登录请求次数过多 | 停止重试，等待限流恢复，必要时重启 `wechat-server` |
| wechat-server 网页提示 Token 无效 | 清理浏览器站点数据并重新登录，不删除数据目录 |
| 重启后机器人离线 | 分别检查协议服务端、客户端、Redis 和登录态，不立即重新扫码 |
| 业务消息先被 AI 回复 | 检查插件顺序以及 `handled=true` 语义 |
| 内置 AI 和网关重复回复 | 检查前置插件是否正确停止后续插件 |
