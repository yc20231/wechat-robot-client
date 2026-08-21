# Dated Static Safety Poster Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 每天 09:00 优先发布对应日期的已审核静态安全海报，并在没有有效静态文件时自动恢复现有动态海报。

**Architecture:** `pkg/safetyreminder` 提供独立的日期静态 PNG 读取与校验函数；`service.SafetyReminderService` 在发送前调用它，命中时直接上传静态字节，未命中或校验失败时调用现有动态 `Preview`。配置只增加一个可选静态目录，现有群、Cron、Redis 去重、主题库和接口保持不变。

**Tech Stack:** Go 标准库、现有 Gin/Redis/消息服务、JSON、Docker、SSH。

## Global Constraints

- 只在 2026-08-22 至 2026-09-19 存放静态文件，共 29 张；不补发 2026-08-21。
- 2026-09-20 起因对应文件不存在而自动使用现有动态渲染。
- 保持 5 个正式群、`0 9 * * *`、周末发送和 `weather_enabled: false` 不变；预览群不加入配置。
- 不调用正式发送接口做部署测试，不删除主题库、预览目录、Redis 数据或微信登录状态。
- 静态文件损坏、为空、不是普通文件或无法读取时记录错误并回退动态渲染。
- 保留任务开始前所有未提交修改；提交只包含本功能文件。

---

### Task 1: 增加静态海报目录配置

**Files:**
- Modify: `pkg/safetyreminder/config.go`
- Test: `pkg/safetyreminder/config_test.go`

**Interfaces:**
- Produces: `Config.StaticPostersDir string`，JSON 字段 `static_posters_dir`。

- [ ] **Step 1: 写配置失败测试**

在 `TestLoadConfigFile` 的 JSON 中加入 `"static_posters_dir":" /data/skills/safety-reminder-static "`，并断言读取结果等于 `/data/skills/safety-reminder-static`。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./pkg/safetyreminder -run TestLoadConfigFile -count=1`

Expected: FAIL，因为严格 JSON 解码器拒绝未知字段。

- [ ] **Step 3: 实现最小配置字段**

在 `Config` 增加 `StaticPostersDir string \`json:"static_posters_dir"\``，并在 `LoadConfigFile` 中执行 `config.StaticPostersDir = strings.TrimSpace(config.StaticPostersDir)`。

- [ ] **Step 4: 运行配置测试**

Run: `go test ./pkg/safetyreminder -run 'TestLoadConfigFile|TestInvalidConfig' -count=1`

Expected: PASS。

---

### Task 2: 按日期安全读取静态 PNG

**Files:**
- Create: `pkg/safetyreminder/static_poster.go`
- Create: `pkg/safetyreminder/static_poster_test.go`

**Interfaces:**
- Produces: `LoadStaticPoster(date time.Time, directory string) (pngBytes []byte, found bool, err error)`。
- Contract: 空目录或文件不存在返回 `(nil, false, nil)`；有效 PNG 返回 `(bytes, true, nil)`；存在但无效的目标返回错误。

- [ ] **Step 1: 写静态读取失败测试**

使用 `t.TempDir()` 写入 `2026-08-22.png`，内容以 PNG 签名开头，断言返回字节不变且 `found=true`。另写表格测试覆盖空目录、缺失文件、零字节、错误签名和同名目录。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./pkg/safetyreminder -run TestLoadStaticPoster -count=1`

Expected: FAIL，`LoadStaticPoster` 尚不存在。

- [ ] **Step 3: 实现静态读取**

`static_poster.go` 定义 PNG 签名 `\x89PNG\r\n\x1a\n`，使用 `filepath.Join(directory, date.Format("2006-01-02")+".png")`，依次执行 `os.Stat`、`Mode().IsRegular()`、`os.ReadFile` 和 `bytes.HasPrefix`。存在但无效的文件错误必须包含目标路径；不存在只表示未命中。

- [ ] **Step 4: 运行包测试**

Run: `go test ./pkg/safetyreminder -count=1`

Expected: PASS。

---

### Task 3: 发送前静态优先并动态回退

**Files:**
- Modify: `service/safety_reminder.go`
- Create: `service/safety_reminder_test.go`

**Interfaces:**
- Produces: `func (s *SafetyReminderService) preparePoster(date time.Time, config safetyreminder.Config) ([]byte, string, error)`。
- Returns: 图片字节、日志用重点标题、仅当静态和动态路径都失败时返回错误。

- [ ] **Step 1: 写选择逻辑失败测试**

临时目录有有效 PNG 时断言 `preparePoster` 原样返回并将标题设为 `静态审核海报`。缺失日期时断言返回动态 PNG 和非空标题。无效 PNG 时捕获日志，断言包含 `静态海报读取失败` 且仍返回动态 PNG。

- [ ] **Step 2: 运行测试并确认失败**

Run: `go test ./service -run TestPrepareSafetyReminderPoster -count=1`

Expected: FAIL，`preparePoster` 尚不存在。

- [ ] **Step 3: 实现最小选择逻辑**

实现静态读取；有效时返回静态字节和 `静态审核海报`。静态错误用 `[SafetyReminder] 静态海报读取失败，回退动态生成` 记录日期和错误；未命中或错误时调用 `s.Preview(date, config.TopicsFile)`。将 `Send` 原来的 `s.Preview` 调用改为 `preparePoster`，其余待发送群、Redis 键和上传循环不变。

- [ ] **Step 4: 运行聚焦测试与相关包测试**

Run: `go test ./service -run TestPrepareSafetyReminderPoster -count=1`

Run: `go test ./pkg/safetyreminder ./common_cron ./controller -count=1`

Expected: PASS。

---

### Task 4: 文档、格式化与本地验证

**Files:**
- Modify: `docs/wechat-bot-fnos-deploy.md`
- Modify: `.deploy/safety-reminder.example.json`

**Interfaces:**
- Documents: `static_posters_dir`、日期命名、缺失回退和只读预览验证。

- [ ] **Step 1: 更新配置示例和文档**

配置示例加入 `"static_posters_dir": "/data/skills/safety-reminder-static"`。说明只放需要覆盖的日期文件，缺失日期使用动态模板；部署预览不得调用 `/send`。

- [ ] **Step 2: 格式化并运行验证**

Run: `gofmt -w pkg/safetyreminder/config.go pkg/safetyreminder/config_test.go pkg/safetyreminder/static_poster.go pkg/safetyreminder/static_poster_test.go service/safety_reminder.go service/safety_reminder_test.go`

Run: `go test ./pkg/safetyreminder ./service ./common_cron ./controller -count=1`

Run: `go vet ./pkg/safetyreminder ./service ./common_cron ./controller`

Expected: PASS。

- [ ] **Step 3: 审查差异并提交功能**

确认差异未修改群 ID、Cron、Redis key 或天气开关。只暂存本任务涉及文件，提交 `feat: publish approved safety posters by date`。

---

### Task 5: 部署 29 张海报与客户端逻辑

**Files:**
- Source: `output/preview-30d/batch-01..06/posters/*.png`
- Server create: `/data/skills/safety-reminder-static/2026-08-22.png` through `2026-09-19.png`
- Server modify: `/data/skills/.safety-reminder.json`
- Server replace: `/app/wechat-robot-client`

**Interfaces:**
- Runtime config: `static_posters_dir=/data/skills/safety-reminder-static`。

- [ ] **Step 1: 建立并验证 29 张部署清单**

生成排序清单，排除 `2026-08-21.png`，断言数量 29、首末日期正确、日期连续、尺寸全部 `1279x1920`、SHA-256 全部唯一。

- [ ] **Step 2: 构建 Linux 客户端并验证文件**

Run: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w -X main.Version=dated-safety-posters-20260821' -o /private/tmp/wechat-robot-client-dated .`

Run: `file /private/tmp/wechat-robot-client-dated`

Expected: Linux x86-64 静态可执行文件。

- [ ] **Step 3: 上传并校验暂存文件**

使用 tar/scp 上传 29 张图片及二进制。服务器解包到新暂存目录，逐个对比本地清单 SHA-256 后才复制到持久化 skills 目录。

- [ ] **Step 4: 备份生产状态并切换**

记录当前镜像 ID并打标签 `rollback-before-dated-safety-posters`。备份 `.safety-reminder.json` 到带时间戳文件。用 `jq` 只增加 `static_posters_dir`，验证 enabled、Cron、5 群、周末、天气和主题路径未变化。

把新二进制复制为容器内 `/app/wechat-robot-client.new`，设为 `0755` 后原子移动到正式路径并重启当前容器；不删除或重建容器。

- [ ] **Step 5: 只读验证并固化镜像**

检查容器运行、版本日志和任务注册。通过带 Token 的 `/preview` 获取 `2026-08-22`、`2026-09-19` 和 `2026-09-20`：前两张 SHA-256 必须与静态源图一致，9 月 20 日必须是有效 PNG 且不属于静态集合。不调用 `/send`。

验证后 `docker commit` 当前容器为 `jiqiren/wechat-robot-client:dated-safety-posters-20260821`，再标记为上游 `latest`，保证后台未来重建仍使用新逻辑。

- [ ] **Step 6: 最终生产核对与回退记录**

断言静态目录恰好 29 个指定日期 PNG；配置目标数 5、Cron `0 9 * * *`、周末 `true`、天气 `false`；Redis 中没有未来日期去重键；日志无 panic 或初始化失败。记录回滚镜像标签和配置备份路径。

若任何验证失败：恢复配置备份，将回滚镜像重新标记为 `latest`，恢复旧二进制或从回滚镜像重建当前客户端，然后确认原动态预览和 Cron 注册正常。不要发送或补发图片。
