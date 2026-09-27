# 生产安排单客户群通知（第一版）

## 范围和默认行为

本次只修改本地源码，不更改飞牛、数据库或微信登录状态。部署前确认仓库商业授权。

数据流：Electron 预览确认 → Go 后端 outbox → 飞牛 business-gateway 主动领取 → 机器人客户端 → 微信群。飞牛不需要公网入站端口。

- 保存生产单、确认开单不会自动发送。发送前必须手动勾选行并二次确认。
- 草稿单也允许手动发送，无需先确认开单；发送不改变生产单状态。作废单或未知状态禁止发送。此规则已随后端v430发布。
- 规格仅移除末尾 `=数字` 厚度部分；`刀100` / `热100` / `100` 提取个数，含多个数字等歧义时留空。
- v431移除叠数输入与校验，第一条数量行格式为 `35个*78包50斤`。旧通知草稿的 `stacks` 字段忽略；历史任务消息保持原样，行标识与防重规则不变。
- 包数默认取现有 `pack_per_bundle`（包装方式第二个数字子列），重量取 `weight_per_carton`，单位斤。两项在预览中可纠正。
- v432新前端按同一生产单内的精确客户代号和目标群合并：第一条以空行分隔选中产品，第二条下单说明仅一次。弹窗按客户分组、产品紧凑表格编辑，显示客户数、产品数及消息数；不跨客户、群或生产单合并。说明不一致须先手动统一，合并第一条超过6000字节禁止入队并提示分批。
- 新请求显式提交 `group_by_customer=true`，后端返回 `grouping_supported=true` 才允许新前端发送；旧客户端未提交该字段时仍按原行发送。新任务 `payload.items` 保存每款产品的 `row_key/product_code/form`，防重及每次发送前校验覆盖所有产品，兼容旧单行历史。
- 仅运维明确授权可把指定已发送记录标记 `released`：保留原消息、回执及数量，不自动入队，不表示微信群消息被撤回。其他记录仍正常防重，禁止把不确定结果当作未发送。
- 通知字段单独保存，不影响打印。第一条由后端按预览字段生成，第二条可编辑。
- 客户代号在后端精确查询，保留前导零，不依赖前端客户列表分页。
- 客户群绑定以网关为准；同时要求通知群白名单。一客户多绑定、未绑定、禁用客户均不能直接发送。
- 单网关、单机器人部署模式；不要让多个不同机器人共用这一套通知 Token/后端桥接记录。

## 安全部署顺序（需操作人另行执行）

1. 备份后端数据库及飞牛当前应用版本、网关数据卷。不要删除 `business-gateway-data`。
2. 更新后端代码，正常启动会执行 `0020_production_notices.sql` 迁移。新增桥接快照、通知草稿、通知任务三张表；不修改生产单表。迁移支持 `DB_PREFIX`。
3. 此时后端 `PRODUCTION_NOTICE_TOKEN` 保持空、`PRODUCTION_NOTICE_REUSE_EXISTING_AUTH=false`，网关 `PRODUCTION_NOTICE_ENABLED=false`。更新 Electron 后可以使用预览、复制和通知草稿，但不能真实发送。
4. 现有部署可复用已有认证，无需申请或生成微信密钥：ERP↔网关复用后端 `BOT_API_TOKEN` / 网关 `BOT_TOKEN`；网关↔机器人复用 `INTERNAL_ROUTE_TOKEN` / 客户端 `.business-gateway.json` 的 `token`。这两条链路的凭据仍须不同且各至少 32 字符。复用会让已有凭据增加通知接口权限，因此必须显式开启；凭据不放进 Electron、不输出到日志。
5. 在机器人已有 Skills 挂载目录放 `.production-notice.json`，容器内通常是 `/data/skills/.production-notice.json`，只需写复用开关，不复制密钥：

   ```json
   { "reuse_existing_auth": true }
   ```

   宿主机路径以现有容器挂载为准，文件权限限制为 `600`。管理后台自动创建容器有环境变量白名单，因此优先使用挂载文件。也支持环境变量 `PRODUCTION_NOTICE_REUSE_EXISTING_AUTH=true`；环境变量可用 `false` 覆盖文件中的复用开关。复用时按现有业务路由优先级读取 `BUSINESS_GATEWAY_URL` / `BUSINESS_GATEWAY_TOKEN` 或 `BUSINESS_GATEWAY_CONFIG_FILE`（默认 Skills 目录下 `.business-gateway.json`）。没有明确开启复用且没有独立 Token、配置无效或 Token 太短时接口拒绝访问。

   如需隔离权限，仍可配置独立 Token：客户端 `PRODUCTION_NOTICE_ROBOT_TOKEN` 优先，其次 `.production-notice.json` 的 `token` 字段，最后才是显式复用。后端和网关的独立通知 Token 同样优先于复用。独立 Token 不能放进前端。
6. 更新机器人客户端代码/镜像；**必须包含本次新的受保护发送接口及回执校验**。不使用旧 `/message/send/text` 作为成功确认依据。保持现有配置和数据卷。
7. 后端设置 `PRODUCTION_NOTICE_REUSE_EXISTING_AUTH=true`，保持 `PRODUCTION_NOTICE_TOKEN` 为空；飞牛网关按 `.env.example` 设置：

   ```dotenv
   PRODUCTION_NOTICE_ENABLED=true
   PRODUCTION_NOTICE_REUSE_EXISTING_AUTH=true
   PRODUCTION_NOTICE_TOKEN=
   PRODUCTION_NOTICE_ROBOT_URL=http://<机器人客户端容器名>:<客户端监听端口>
   PRODUCTION_NOTICE_ROBOT_TOKEN=
   PRODUCTION_NOTICE_ALLOWED_GROUPS=<测试群真实ID>@chatroom
   ```

   `BACKEND_URL` 沿用查库存配置（后端根 URL，不带 `/api`）；两个服务之间跨公网必须使用 HTTPS。机器人 URL 指向客户端，不是协议服务端或管理后台。新接口只在内部 Docker 网络使用，不能裸露到公网。
8. 更新并重建 business-gateway，保留卷。worker 每 5 秒轮询，先同步客户群绑定和就绪状态，再领一条任务。后端超过 45 秒没有心跳就禁止新发送。
9. 用专用测试客户/测试生产单测试。测试群绑定不能临时覆盖真实客户原绑定。完成验收后由操作人明确增加正式群 ID 白名单。

## 状态与恢复

- `pending`：待领取，提交起 10 分钟有效。
- `claimed` / `sending`：执行中；同一时刻只有一个任务，按第一条→确认结果→第二条执行。
- `sent`：两条都获得协议回执，不代表客户已读。
- `blocked`：发送前检查失败，没有启动当前条发送。核对后可继续，只补尚未确认的消息。
- `expired`：待领取任务过期。必须在 ERP 点击“核对后继续”，不能上线后突然补发过期订单。
- `unknown`：请求/回执/执行租约结果不明。**不自动重试，也不提供一键重发**。人工查看微信群处理；第一条已确认时不会继续发第二条，防止发生重复。

HTTP 超时不等于没发出去。发送开始前，后端已持久化 `sending`；worker 崩溃或回传失败后，租约过期只变成 unknown，不会重新派发。两条分别计数；确认第一条后只允许继续第二条。

任务保留最终消息快照、客户、群、创建人、创建时间、发送计数和异常状态；ERP 的现有请求审计记录提交/继续操作人。领取凭据不返回给 Electron。

现有生产明细保存会删除重建记录。本版用“通知源字段内容＋相同行出现序号”生成行快照键，不使用会变化的明细 ID；重排/重复保存不导致重发。修改源字段会生成新快照，所以界面提醒先核对已有通知记录；不会自动发送变更通知。生产单版本或群绑定发生变化时旧任务停止，不能直接继续旧消息。重复且完全相同的行按出现序号区分。

## 接口契约

ERP（员工 JWT + `production.manage`）：

- `GET /api/admin/production-schedules/:id/notices`：预览、草稿、群快照和历史。
- `POST .../notices/drafts`：保存通知字段，`revision + rows[{row_key, group_id, form}]`。
- `POST .../notices`：校验保存版本、唯一群绑定、字段完整性后创建任务；重复行拒绝。
- `POST .../notices/:task/retry`：对 blocked/expired 明确再次确认。

飞牛→ERP（`X-Bot-Token` + `User-Agent: bot-mcp/...`，可显式复用现有 Bot 凭据）：

- `POST /api/bot/production-notices/heartbeat`：`bindings, ready`。
- `POST .../claim`：领取一个任务和随机租约。
- `POST .../:task/begin`：`lease_token, index`，发送前持久化意图。
- `POST .../:task/result`：`lease_token, index, outcome`；outcome 为 sent/blocked/unknown。

网关→客户端（`X-Production-Notice-Token`，可显式复用现有内部路由凭据）：

- `GET /api/v1/robot/production-notice/health`。
- `POST /api/v1/robot/production-notice/send`：`to_wxid, content`；验证协议总体结果、单条 Ret 和消息 ID 后才返回 `accepted:true`。

## 本地验证（不发微信）

- 后端：`go test ./internal/modules/productionnotices ./internal/modules/productionschedules ./internal/platform/migrations ./internal/platform/config ./cmd/api`。
- Windows 独立 MySQL：`node scripts/production-notice-integration.mjs`（在 houduan 内运行；需要本机 MySQL 8.4 和 `.tools/go1.26.5`；仅随机回环端口和新建临时库，不读取 .env）。
- 网关：`go test ./internal/notices ./internal/config ./internal/route ./internal/httpapi`。
- 客户端：`go test ./pkg/robot ./controller ./service -run 'TestProductionNotice|TestValidateProductionNotice'`。
- Electron：`node --import tsx --test src/renderer/tests/production-notice.test.ts`、`pnpm exec vue-tsc --noEmit -p src/renderer/tsconfig.json`。
- 预览交互与截图：`node scripts/verify-production-notice-preview.mjs`（Electron_houtai 内运行，默认使用本机 Edge；`NOTICE_TEST_BROWSER=chrome` 可改 Chrome）。只挂载真实组件和 mock API，不加载 ERP 凭据。验证不再显示或提交叠数、预览文本、二次确认、单次提交、已发送行禁选和离线拦截。

本地验证备注：网关的全量 `go test ./...` 在 Windows 下有原有 `internal/audit/TestFileLoggerWritesPrivateJSONLines` 的 Unix `0600` 权限断言失败（Windows 报 `0666`）；通知、配置、路由、HTTP 定向测试不受影响。

回滚时先关闭网关 worker，再清空后端通知 Token 并设置 `PRODUCTION_NOTICE_REUSE_EXISTING_AUTH=false`；客户端也关闭复用并移除独立通知 Token（如有）。不要清空现有查库存凭据。保留三个任务表、网关数据卷与审计记录。关闭通知不会影响现有查库存。
