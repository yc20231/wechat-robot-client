# 业务网关设计与 ThinkPHP 接口契约

本文档描述当前确定的业务方案：

```text
hp0912/wechat-robot-client
    -> BusinessRouterPlugin
       ├─ 业务消息 -> business-gateway -> ThinkPHP
       └─ 非业务消息 -> hp0912 内置 AI
    -> Webhook（审计/同步）
```

MCP 不再是库存、订单等实时业务的主入口。hp0912 内置 AI 负责非业务消息；业务消息必须先经过前置路由和权限校验。

## 0. 实现状态

截至 2026-07-24：

- hp0912 平台、微信登录、内置 AI 已部署并验证。
- 自定义客户端 AI 文本尾注已部署并验证。
- `BusinessRouterPlugin` 和 `business-gateway` 已部署到飞牛，Token、权限、故障闭合、AI 放行和 AI 尾注已经实测。
- 固定所有者、动态管理员、群内绑定、风险操作确认和审计功能已部署并通过微信测试。
- 权限感知的`菜单`与`帮助`已部署；旧`业务帮助`作为兼容别名保留。
- `菜单`与`帮助`已列出文生图、引用图片修改、机器人生成图续改和双图替换流程。
- ThinkPHP `/api/bot/health`、`/api/bot/inventory`、`/api/bot/customers/resolve` 和独立 Bot Token 鉴权已经连通验证。
- 旧 `wangzhan/bot-mcp` 已停用但暂不删除，只保留作迁移参考。

真实客户群接入前仍应逐群核对 `customer_code`，并先在测试群验证权限和库存隔离。

## 1. 业务规则

### 客户群

每个客户群固定绑定一个 `customer_code`：

```text
群 A -> 270
群 B -> 300
```

群 A 只能查询 270 的数据，不能通过消息中的数字、AI 参数或提示词查询 300。

### 管理员群

管理员群拥有全部业务模块和测试模块，但必须同时满足：

```text
群类型 = admin
发送者 wxid 是全局固定所有者、动态根管理员或动态普通管理员
```

只满足其中一个条件也不能跨客户查询。

管理员群不绑定某个 `customer_code`。只有固定所有者可以在目标群内首次绑定、改绑或解绑管理员群；改绑和解绑需要确认，首次绑定立即生效。受保护的 HTTP 管理 API 仍可用于维护。

### 全局管理员

权限分为三级：

| 角色 | 持久化 | 权限 |
|---|---|---|
| 固定所有者 | `OWNER_WXIDS` 环境变量 | 不可修改或移除；拥有全部管理权限 |
| 动态根管理员 | `/data/admins.json` | 可添加/移除动态根管理员和普通管理员；可管理客户群绑定 |
| 动态普通管理员 | `/data/admins.json` | 可在管理员群跨客户查询 |

管理员身份是全局身份，不只针对发出指令的群。移除动态根管理员会先降为动态普通管理员；之后可再移除普通管理员权限。角色和群绑定变更均追加到 `/data/audit.jsonl`。

高风险写操作采用二次确认：移除/降级管理员、改绑或解绑群时，同一操作者、同一群、同一操作参数必须在 `CONFIRMATION_TTL_SEC` 内确认，默认 300 秒。添加管理员和首次绑定立即生效。待确认状态只保存在内存中，网关重启后必须重新发起。

### AI

处理顺序固定为：

```text
BusinessRouterPlugin
  -> 业务消息：business-gateway -> 权限校验 -> 业务模块 -> 直接回复
  -> 非业务消息：hp0912 内置 AI
```

AI 不得回答实时库存、订单、余额、排期和客户数据。后端请求失败时也不能让 AI 猜测结果。

AI 不是第一期放弃的功能。第一期继续使用 hp0912 内置 AI，负责非业务闲聊、知识库和记忆。客户群和管理员群仍然可以启用 AI，但业务消息必须由前置路由拦截，不能先进入 AI。

图片生成与编辑由内置 AI 激活 `text-to-image` Skill：普通文字请求执行文生图；引用图片时编辑引用目标图；先发送参考图再引用目标图时可执行双图替换。双图参考图仅从同一会话、同一发送者最近 5 分钟的图片中选择。

OpenAI 兼容中转站的图片尺寸由 Skill 自动归一化。未固定配置尺寸时，文生图按用户比例选择横版、竖版或方形尺寸，引用编辑则优先读取目标 PNG/JPEG 的宽高；不会把中转站可能拒绝的 `size=auto` 原样发送。

### AI 配置位置

AI 配置仍然在 hp0912 管理后台维护：API 地址、API Key、模型、系统提示词、知识库和记忆。系统提示词必须明确禁止猜测实时库存、订单、余额和排期数据；业务安全不能只依赖提示词，必须依赖前置路由。

## 2. 群绑定数据模型

早期可以使用 JSON 文件，正式环境建议迁移到 ThinkPHP 后台和数据库。

目标结构：

```go
type GroupType string

const (
    GroupTypeCustomer GroupType = "customer"
    GroupTypeAdmin    GroupType = "admin"
)

type GroupBinding struct {
    GroupID      string    `json:"group_id"`
    GroupName    string    `json:"group_name"`
    Type         GroupType `json:"type"`
    CustomerCode string    `json:"customer_code,omitempty"`
    Enabled      bool      `json:"enabled"`
}
```

管理员群示例：

```json
{
  "group_id": "admin-group@chatroom",
  "group_name": "机器人管理员群",
  "type": "admin",
  "enabled": true
}
```

客户群示例：

```json
{
  "group_id": "customer-270@chatroom",
  "group_name": "270客户群",
  "type": "customer",
  "customer_code": "270",
  "enabled": true
}
```

## 3. BusinessRouterPlugin 输入

`jiqiren` 客户端中的 `BusinessRouterPlugin` 在 hp0912 内置 AI 之前运行，同步调用：

```text
POST http://business-gateway:8080/internal/business/route
```

请求字段：

```text
robot_wxid
robot_code
group_id
sender_wxid
message_id
content
is_at_me
mentioned_wxids       MessageSource.atuserlist 中的真实 wxid 列表
```

请求使用 `X-Internal-Route-Token`，其值必须与客户端 `BUSINESS_GATEWAY_TOKEN` 一致。

响应状态：

```text
handled=true      已处理，客户端发送 reply 并阻止 AI
handled=false     非业务消息，继续内置 AI
handled=true + error 业务失败，发送固定错误并阻止 AI
reply_at_wxids        回复时额外 @ 的真实 wxid 列表
```

业务错误不能返回 `handled=false`，否则会让 AI 接管实时业务问题。

## 4. hp0912 Webhook 输入

hp0912 Webhook 发送同步消息批次。网关关注 `AddMsgs`：

```text
Appid
Wxid                 机器人 wxid
AddMsgs[].FromUserName
AddMsgs[].ToUserName
AddMsgs[].Content
AddMsgs[].MsgType
AddMsgs[].NewMsgId
```

不同消息类型和 XML 内容见 hp0912 的 [`message_callback.md`](https://github.com/hp0912/wechat-robot-client/blob/main/message_callback.md)。

消息去重键：

```text
dedup_key = Appid + ":" + NewMsgId
```

网关必须过滤机器人自己发出的消息，并按配置判断是否必须 @机器人后才处理。

## 5. 业务模块权限

模块注册应使用稳定/实验两种状态：

```text
stable       客户群和管理员群都可见
experimental 仅管理员群 + 管理员 wxid 可用
```

权限类型：

```text
customer
admin
```

有效权限计算：

```text
客户群       -> customer
管理员群     -> admin，但仍需管理员 wxid 白名单
```

一期模块：

```text
help       stable
inventory  stable
status     experimental
```

后续模块：

```text
order
balance
production_schedule
shipment
statement
```

## 6. ThinkPHP 接口契约

所有机器人接口使用独立 token，不复用员工或客户 JWT：

```http
X-Bot-Token: <BOT_API_TOKEN>
User-Agent: bot-mcp/business-gateway-1.0
```

`bot-mcp/` 前缀用于兼容现有 ThinkPHP `BotTokenMiddleware` 的调用来源校验；服务本身仍是新的前置业务网关，不会恢复旧 MCP 路径。

### 健康检查

```text
GET /api/bot/health
```

### 查询库存

```text
GET /api/bot/inventory
```

查询参数：

| 参数 | 必填 | 说明 |
|---|---|---|
| `customer_code` | 是 | 已经经过网关身份校验的客户代号 |
| `keyword` | 否 | 货号、品名、规格、颜色、花纹模糊匹配 |
| `product_code` | 否 | 货号模糊匹配，与 `keyword` 叠加 |
| `limit` | 否 | 默认 20，硬上限 50 |

成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "customer_code": "270",
    "customer_name": "示例客户",
    "summary": {
      "count": 1,
      "total_carton_qty": 2,
      "total_weight_jin": 30
    },
    "items": []
  }
}
```

后端仍然必须再次按 `customer_code` 隔离查询，不能完全信任网关传入值。

### 校验客户代号

```text
GET /api/bot/customers/resolve?customer_code=270
```

成功响应只返回绑定所需的最少字段：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "exists": true,
    "customer_code": "270",
    "customer_name": "示例客户"
  }
}
```

接口先查启用的 `customers` 记录，再兼容仅存在于启用库存历史中的线下客户。客户已明确禁用时返回 `exists=false`，不能因存在历史库存而重新通过校验。

## 7. 新增业务模块的位置

以订单为例：

```text
backend/app/controller/BotOrderController.php
backend/app/service/BotOrderService.php
backend/route/app.php

business-gateway/internal/backend/order.go
business-gateway/internal/modules/order/
business-gateway/internal/modules/registry.go
```

正常情况下不需要修改：

```text
internal/adapter/
internal/identity/
internal/dedup/
```

## 8. 管理接口

受保护的 HTTP 管理接口使用独立的 `ADMIN_TOKEN`：

```text
GET    /admin/groups
POST   /admin/groups
PUT    /admin/groups/{group_id}
DELETE /admin/groups/{group_id}
```

群内管理指令如下。添加管理员和首次绑定立即生效；移除/降级、改绑和解绑需要二次确认：

```text
菜单
帮助（业务帮助为兼容别名）

管理员列表
添加 @成员 根管理员
移除 @成员 根管理员
确认移除 @成员 根管理员
添加 @成员 管理员
移除 @成员 管理员
确认移除 @成员 管理员

查看群绑定
绑定客户 <customer_code>
改绑客户 <customer_code>
确认改绑 <customer_code>
解绑客户
确认解绑客户 <customer_code>

绑定管理员群
改绑/确认改绑管理员群
解绑/确认解绑管理员群
```

管理员目标只接受微信消息协议 `MessageSource.atuserlist` 中的真实 wxid。昵称文字和手工输入的 `@名字` 不能成为授权依据。

## 9. 测试要求

自动化测试已覆盖：

- [x] 270 客户群不能查询 300
- [x] 管理员群普通成员不能跨客户查询
- [x] 管理员群白名单成员可以跨客户查询
- [x] 实验模块对客户群拒绝，对管理员开放
- [x] 重复内部业务消息只查询和回复一次
- [x] Webhook 按 `Appid + NewMsgId` 去重
- [x] 未 @机器人时不处理群业务消息
- [x] 后端失败时不进入 AI 编造业务答案
- [x] 业务消息先被前置路由处理，不能先触发 hp0912 AI
- [x] 非业务消息继续进入 hp0912 内置 AI
- [x] AI 文本回复带有“本消息由AI生成回复”尾注
- [x] 机器人自己发送的消息不会再次触发业务
- [x] 只有真实且唯一的 @ 目标可用于管理员变更
- [x] 固定所有者不能被修改或移除
- [x] 动态根管理员可管理动态根管理员和普通管理员
- [x] 管理员变更全局持久化，网关重启后仍有效
- [x] 客户群绑定、改绑、解绑和客户代号校验
- [x] 只有固定所有者可以管理管理员群
- [x] 高风险移除/改绑/解绑操作的同操作者、同群、5 分钟确认窗口和过期拒绝
- [x] 添加管理员和首次绑定立即生效
- [x] 管理员与群绑定变更写入 JSONL 审计日志
- [x] 菜单和帮助在未绑定群可用；菜单按当前群过滤，帮助按角色列出完整功能与适用条件

## 10. 与客户端镜像的关系

`BusinessRouterPlugin` 和 AI 尾注都属于自定义客户端代码。管理后台当前把客户端镜像固定为：

```text
registry.cn-shenzhen.aliyuncs.com/houhou/wechat-robot-client:latest
```

飞牛部署通过本地标签把经过验证的自定义镜像映射为 `latest`，并额外保留官方回滚标签。禁止直接点击管理后台“更新镜像”，否则自定义业务路由和 AI 尾注都会被官方镜像覆盖。

完整构建、切换和回滚步骤见 `docs/wechat-bot-fnos-deploy.md`。
