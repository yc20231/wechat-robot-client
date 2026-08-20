# 8 月 21 日静态安全海报定时发布实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在飞牛服务器于 2026-08-21 09:00 CST 将已审核的消防安全检查 PNG 发送到现有 5 个目标群，且不触发旧模板的重复发送。

**Architecture:** 将静态 PNG 放入客户端已挂载的 `/data/skills`。临时关闭内置安全提醒 Cron，由 root crontab 调用一次性脚本逐群请求客户端本地图片发送接口；脚本结束后恢复原配置并重启客户端，使次日任务照常注册。

**Tech Stack:** SSH、Docker、cron、curl、jq、微信机器人客户端本地 HTTP API。

## Global Constraints

- 不输出或复制配置中的 `test_token`。
- 不修改 5 个既有目标群 ID、原执行时间或主题库路径。
- 发送前禁用内置 Cron，避免 09:00 重复发布。
- 一次性脚本必须逐群记录结果，并在执行结束后恢复原配置。
- 静态 PNG 必须与本地审核文件 SHA-256 一致。

---

### Task 1: 上传并校验静态海报

**Files:**
- Source: `output/imagegen/fire-safety-poster-2026-08-21.png`
- Create (server): `.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/safety-reminder-2026-08-21.png`

- [ ] **Step 1:** 计算本地 PNG 的 SHA-256 和尺寸。
- [ ] **Step 2:** 通过 SSH 临时目录上传，再以只读权限安装到客户端 skills 挂载目录。
- [ ] **Step 3:** 对比服务器 SHA-256，确认与本地完全一致。

### Task 2: 暂停旧模板的今日 Cron

**Files:**
- Modify (server): `.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/.safety-reminder.json`
- Create (server): 同目录 `.safety-reminder.json.pre-static-2026-08-21`

- [ ] **Step 1:** 使用 `cp -a` 备份原配置。
- [ ] **Step 2:** 仅将 `enabled` 改为 `false`，保留群、Cron、Token 和主题路径。
- [ ] **Step 3:** 重启 `client_xiW55bPyM3D4o6s6`，确认日志显示每日安全提醒未启用。

### Task 3: 安装一次性 09:00 发送任务

**Files:**
- Create (server): `.deploy/local/wechat-robot/xiW55bPyM3D4o6s6/data/skills/send-static-safety-2026-08-21.sh`
- Create at runtime (server): 同目录 `send-static-safety-2026-08-21.log`
- Modify (server): root crontab，添加带唯一标记的单行任务。

- [ ] **Step 1:** 安装脚本，脚本从原配置读取目标群并逐群调用 `/api/v1/robot/message/send/image/local`。
- [ ] **Step 2:** 脚本检查每次响应的 `code == 200`，记录成功或失败，不记录 Token。
- [ ] **Step 3:** 脚本无论发送结果如何都恢复原配置、重启客户端并删除自身 crontab 行。
- [ ] **Step 4:** 用不存在的图片路径调用接口，确认路由返回预期错误且不会发送消息。
- [ ] **Step 5:** 安装 `0 9 21 8 *` 的 root crontab 行并核对服务器时间。

### Task 4: 发送后核验

**Files:**
- Read (server): `send-static-safety-2026-08-21.log`
- Read (server): `.safety-reminder.json`

- [ ] **Step 1:** 09:00 后确认日志中 5 个目标群均为 `sent`。
- [ ] **Step 2:** 确认原配置已恢复为 `enabled: true`，客户端正常运行。
- [ ] **Step 3:** 确认一次性 root crontab 行已删除，避免未来重复执行。
- [ ] **Step 4:** 检查客户端重启日志，确认次日 09:00 的每日安全任务重新注册。
