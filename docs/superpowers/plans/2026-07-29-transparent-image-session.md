# Transparent Image Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with verification checkpoints.

**Goal:** Add a ChatGPT-like image session that routes image requests directly to the image model, preserves user prompts, supports explicit multi-image context, and expires after five minutes of inactivity with a four-minute warning.

**Architecture:** Add an `ImageSessionPlugin` before the existing chat plugins. It uses an `ImageSessionService` backed by Redis to own per-user conversation state and calls a `DirectImageService` that executes the deployed image transport with a JSON result contract; Go sends returned images and updates the session target. Existing chat behavior remains the fallback for messages outside an image session.

**Tech Stack:** Go 1.25, Gin message pipeline, Redis (`go-redis/v9`), GORM message records, existing WeChat `SendAppMessage`/image APIs, deployed Python image transport, OpenAI-compatible image API.

## Global Constraints

- Preserve the user's prompt after removing only WeChat communication fields such as the bot mention and configured trigger prefix.
- Do not add fixed image-command keyword lists or model-generated prompt expansions.
- Text-to-image requests omit `size` and `quality`.
- Image-edit requests use the current target's original size only when it is accepted by the selected model/upstream; never silently downgrade to a fixed size.
- Image-session messages must not enter `activate_skill`, `execute_skill_script`, or the ordinary chat Agent.
- Redis stores session metadata only; input/output binaries exist only in task-scoped temporary files and are deleted after the task.
- Session identity is `robot_id + conversation_id + sender_wxid`; group members never share image state.
- Idle sessions warn at four minutes and expire at five minutes; processing tasks are not warned or expired.
- Existing untracked `.skill-backups/` is unrelated and must not be modified or removed.
- FNOS deployment is separate from this Mac workspace; no claim of deployed behavior until a candidate image is installed and verified on the N100 host.

## File Map

- Create `service/image_session.go`: Redis-backed session model, atomic asset/role operations, TTL and processing state.
- Create `service/image_session_test.go`: session key, numbering, target changes, TTL and processing tests using a Redis test double or miniredis.
- Modify `go.mod`: add `github.com/alicebob/miniredis/v2` as a test dependency.
- Modify `go.sum`: record the test dependency checksums.
- Create `service/direct_image.go`: direct image transport adapter, raw-prompt contract, task-scoped files, structured result parsing and output metadata.
- Create `service/direct_image_test.go`: request construction and result/error contract tests without external network calls.
- Create `service/image_intent_router.go`: route-only `chat`/`image` classification using the configured chat model without tools or prompt rewriting.
- Create `service/image_intent_router_test.go`: strict route parsing, prompt preservation and failure fallback tests.
- Create `plugin/plugins/image_session.go`: image/text routing, session asset ingestion, prompt forwarding and `ctx.Handled` behavior.
- Create `plugin/plugins/image_session_test.go`: routing isolation, prompt preservation, image-session continuation and chat fallback tests.
- Create `service/image_session_expiry.go`: persistent warning scan and session cleanup worker.
- Create `service/image_session_expiry_test.go`: one-warning semantics, group mention/private fallback and processing protection.
- Create `service/quote_message.go`: Type 57 quote XML builder and safe fallback helper.
- Create `service/quote_message_test.go`: XML escaping and required `<refermsg>` fields.
- Modify `startup/plugin.go`: register `ImageSessionPlugin` before `ChatRoomAIChatPlugin` and `FriendAIChatPlugin` paths can run.
- Modify `service/message.go`: add `SendImageMessageByLocalPathWithResult` while keeping the existing error-only wrapper.
- Modify `interface/plugin/message.go`: expose `SendImageMessageByLocalPathWithResult` to the image-session plugin.
- Modify `.deploy/skills/text-to-image-gpt-edit.patch`: remove prompt/negative-prompt/size/quality invention and add explicit target/reference IDs plus JSON output mode.
- Modify `.deploy/skills/text-to-image-auto-url.patch`: pass the same transparent parameters through the image URL fallback.
- Modify `.deploy/skills/text_to_image_edit_transport.py`: keep multipart/image-url retry behavior while returning structured responses for Go.
- Modify `.deploy/skills/test_text_to_image_edit_transport.py`: test transport fallback and the new output contract.
- Modify `docs/wechat-bot-fnos-deploy.md`: document skill installation, image transport contract, FNOS quote-message verification and rollout steps.

### Task 1: Build Redis Image Session State

**Files:**
- Create: `service/image_session.go`
- Test: `service/image_session_test.go`

**Interfaces:**
- `type ImageSessionAsset struct { Number int; MessageID int64; Role string }`
- `type ImageSession struct { SessionID, CreatedBy, ConversationID string; NextNumber, Target, LastOutput int; LastActivityUnix int64; WarningSent bool; State string; Assets []ImageSessionAsset }`
- `func ImageSessionKey(robotID, conversationID, senderWxID string) string`
- `func NewImageSessionService(ctx context.Context) *ImageSessionService`
- `func (s *ImageSessionService) Get(key string) (*ImageSession, error)`
- `func (s *ImageSessionService) AddAsset(key string, createdBy string, messageID int64, role string) (ImageSessionAsset, *ImageSession, error)`
- `func (s *ImageSessionService) SetTarget(key string, number int) (*ImageSession, error)`
- `func (s *ImageSessionService) DeleteAsset(key string, number int) (*ImageSession, error)`
- `func (s *ImageSessionService) Touch(key string) error`
- `func (s *ImageSessionService) BeginProcessing(key string) error`
- `func (s *ImageSessionService) FinishProcessing(key string) error`
- `func (s *ImageSessionService) MarkWarningSent(key string) (bool, error)`

- [ ] **Step 1: Add failing tests for isolated keys and first/subsequent assets.**

```go
func newImageSessionTestService(t *testing.T) (*ImageSessionService, string) {
    server := miniredis.RunT(t)
    previous := vars.RedisClient
    vars.RedisClient = redis.NewClient(&redis.Options{Addr: server.Addr()})
    t.Cleanup(func() {
        _ = vars.RedisClient.Close()
        vars.RedisClient = previous
    })
    return NewImageSessionService(context.Background()), ImageSessionKey("robot", "room@chatroom", "wxid_a")
}

func TestImageSessionKeyIncludesSender(t *testing.T) {
    first := ImageSessionKey("robot", "room@chatroom", "wxid_a")
    second := ImageSessionKey("robot", "room@chatroom", "wxid_b")
    if first == second { t.Fatal("group members must not share a session key") }
}

func TestAddAssetAssignsTargetThenReferences(t *testing.T) {
    svc, key := newImageSessionTestService(t)
    first, session, err := svc.AddAsset(key, "wxid_a", 101, "target")
    if err != nil || first.Number != 1 || session.Target != 1 { t.Fatalf("first asset = %+v %+v %v", first, session, err) }
    second, session, err := svc.AddAsset(key, "wxid_a", 102, "reference")
    if err != nil || second.Number != 2 || session.Assets[1].Role != "reference" { t.Fatalf("second asset = %+v %+v %v", second, session, err) }
}
```

- [ ] **Step 2: Run the focused tests and verify they fail because the session service is absent.**

Run: `go test ./service -run 'TestImageSession(KeyIncludesSender|AddAssetAssignsTargetThenReferences)' -count=1`

Expected: FAIL because `ImageSessionKey`, `ImageSessionService` and the asset/session types do not exist.

- [ ] **Step 3: Implement the Redis JSON state and atomic mutation path.**

Use one Redis key per session with a five-minute idle TTL. Serialize the complete session as JSON. Protect read-modify-write operations with the existing `pkg/distributedlock` helper keyed by the session key. On first `AddAsset`, initialize `session_id`, `created_by`, `next_number=1`, `target=0`, `state="idle"`; assign number 1 as target and all later assets as references. Every successful mutation refreshes `last_activity_unix`, the idle TTL, and clears `warning_sent` except `MarkWarningSent`. `BeginProcessing` changes state and replaces the idle TTL with a 30-minute crash-recovery TTL; `FinishProcessing` restores `idle` and the five-minute TTL.

- [ ] **Step 4: Add tests for target changes, deletion, TTL refresh, warning idempotency and processing.**

```go
func TestMarkWarningSentOnlySucceedsOnce(t *testing.T) {
    svc, key := newImageSessionTestService(t)
    _, _, _ = svc.AddAsset(key, "wxid_a", 101, "target")
    first, err := svc.MarkWarningSent(key)
    if err != nil || !first { t.Fatalf("first mark = %v, %v", first, err) }
    second, err := svc.MarkWarningSent(key)
    if err != nil || second { t.Fatalf("second mark = %v, %v", second, err) }
}

func TestSetTargetAndDeleteAssetKeepExplicitTarget(t *testing.T) {
    svc, key := newImageSessionTestService(t)
    _, _, _ = svc.AddAsset(key, "wxid_a", 101, "target")
    _, _, _ = svc.AddAsset(key, "wxid_a", 102, "reference")
    session, _ := svc.SetTarget(key, 2)
    if session.Target != 2 { t.Fatalf("target = %d", session.Target) }
    session, _ = svc.DeleteAsset(key, 1)
    if session.Target != 2 || len(session.Assets) != 1 { t.Fatalf("session = %+v", session) }
}
```

- [ ] **Step 5: Run the complete session package tests.**

Run: `go test ./service -run ImageSession -count=1`

Expected: PASS.

- [ ] **Step 6: Commit the session state unit.**

```bash
git add service/image_session.go service/image_session_test.go go.mod go.sum
git commit -m "feat: add redis image session state"
```

### Task 2: Make the Image Transport Transparent and Structured

**Files:**
- Create: `service/direct_image.go`
- Test: `service/direct_image_test.go`
- Modify: `.deploy/skills/text-to-image-gpt-edit.patch`
- Modify: `.deploy/skills/text-to-image-auto-url.patch`
- Modify: `.deploy/skills/text_to_image_edit_transport.py`
- Modify: `.deploy/skills/test_text_to_image_edit_transport.py`

**Interfaces:**
- `type DirectImageRequest struct { Prompt string; Model string; TargetMessageID int64; ReferenceMessageIDs []int64; TargetSize string }`
- `type DirectImageOutput struct { LocalPath string; Width int; Height int; RemoteURL string }`
- `type DirectImageResult struct { Outputs []DirectImageOutput; RequestedSize string; ReturnedSize []string; Cleanup func() }`
- `func (s *DirectImageService) Generate(ctx context.Context, request DirectImageRequest) (*DirectImageResult, error)`

- [ ] **Step 1: Add failing tests for prompt and parameter transparency.**

```go
func TestBuildDirectImagePayloadOmitsSizeAndQualityForTextGeneration(t *testing.T) {
    payload := buildDirectImagePayload(DirectImageRequest{Prompt: "画一只穿宇航服的柴犬"})
    if _, ok := payload["size"]; ok { t.Fatal("text generation must omit size") }
    if _, ok := payload["quality"]; ok { t.Fatal("text generation must omit quality") }
    if payload["prompt"] != "画一只穿宇航服的柴犬" { t.Fatalf("prompt changed: %#v", payload["prompt"]) }
}

func TestBuildDirectImagePayloadUsesTargetSizeOnlyForEdit(t *testing.T) {
    payload := buildDirectImagePayload(DirectImageRequest{Prompt: "帮我加水印 仅限云盛公司内部专用", TargetMessageID: 101, TargetSize: "1920x1344"})
    if payload["size"] != "1920x1344" { t.Fatalf("size = %#v", payload["size"]) }
    if payload["prompt"] != "帮我加水印 仅限云盛公司内部专用" { t.Fatalf("prompt changed: %#v", payload["prompt"]) }
}
```

- [ ] **Step 2: Run the focused tests and verify failure.**

Run: `go test ./service -run 'TestBuildDirectImagePayload' -count=1`

Expected: FAIL until the direct transport payload builder exists.

- [ ] **Step 3: Change the deployed transport contract.**

Update the patch and transport helper so the script accepts explicit `--target-message-id` and `--reference-message-ids` values, does not query recent images by time, does not prepend negative prompts or creative role instructions, and distinguishes generation from edit by the presence of target inputs. For generation omit `size` and `quality` fields. For edits send the original target dimensions only when supplied by Go. The script must return JSON containing output paths/URLs and dimensions instead of sending images itself; retain the existing multipart-to-`image_url` retry only for the single-image fallback.

The output contract is:

```json
{"outputs":[{"path":"/tmp/wechat-output-image-1.png","width":1920,"height":1344}],"requested_size":"1920x1344"}
```

Never shell-concatenate user text. Pass prompt and IDs as argument-array values or a JSON request file.

- [ ] **Step 4: Implement `DirectImageService.Generate`.**

Create task-scoped temporary directories, download target/reference message images through the existing client endpoint, invoke the deployed transport with structured arguments, decode JSON, and validate output files exist. Return an idempotent `Cleanup` closure on `DirectImageResult`; the plugin must `defer result.Cleanup()` immediately after a successful call and send all outputs before returning. Do not resize or recompress returned files. Propagate upstream errors with request/session context.

- [ ] **Step 5: Add transport-script tests.**

Extend `test_text_to_image_edit_transport.py` to assert single-image fallback behavior, multi-image rejection in URL mode, JSON output parsing, omission of generation size/quality, and exact prompt bytes after UTF-8 round-trip.

- [ ] **Step 6: Run Go and Python focused tests.**

Run: `go test ./service -run DirectImage -count=1`

Run: `python3 -m unittest .deploy/skills/test_text_to_image_edit_transport.py`

Expected: PASS without network access.

- [ ] **Step 7: Commit the direct transport unit.**

```bash
git add service/direct_image.go service/direct_image_test.go .deploy/skills/text-to-image-gpt-edit.patch .deploy/skills/text-to-image-auto-url.patch .deploy/skills/text_to_image_edit_transport.py .deploy/skills/test_text_to_image_edit_transport.py
git commit -m "feat: add transparent direct image transport"
```

### Task 3: Route Image Sessions Before Ordinary Chat

**Files:**
- Create: `plugin/plugins/image_session.go`
- Test: `plugin/plugins/image_session_test.go`
- Create: `service/image_intent_router.go`
- Test: `service/image_intent_router_test.go`
- Modify: `startup/plugin.go`
- Modify: `service/message.go`
- Modify: `interface/plugin/message.go`

**Interfaces:**
- `func NewImageSessionPlugin() plugin.MessageHandler`
- `func (p *ImageSessionPlugin) Match(ctx *plugin.MessageContext) bool`
- `func (p *ImageSessionPlugin) Run(ctx *plugin.MessageContext)`
- `func stripImageTransportFields(content, triggerWord string) string`
- `func buildImageRequestPrompt(content, triggerWord string) string`
- `type ImageTaskLogFields struct { RequestID, SessionID, RawPrompt, SanitizedPrompt, Model, RequestedSize string; TargetMessageID int64; ReferenceMessageIDs []int64; ReturnedSizes []string; Elapsed time.Duration; UpstreamError string }`
- `type ImageIntentRoute string` with constants `ImageIntentChat` and `ImageIntentImage`
- `func (r *ImageIntentRouter) Route(ctx context.Context, prompt string, settings settings.Settings) (ImageIntentRoute, error)`
- `func (s *MessageService) SendImageMessageByLocalPathWithResult(toWxID, imagePath, imageURL string) (*model.Message, error)`

- [ ] **Step 1: Add failing routing tests.**

```go
func TestImageSessionPromptRemovesOnlyTransportFields(t *testing.T) {
    got := stripImageTransportFields("@阳强机器人\u2005帮我加水印 仅限云盛公司内部专用", "")
    if got != "帮我加水印 仅限云盛公司内部专用" { t.Fatalf("got %q", got) }
}
func TestChatFallbackRemainsForNonImageText(t *testing.T) {
    ctx := newImagePluginTextContext("今天天气怎么样")
    plugin := newImageSessionPluginWithRoute(ImageIntentChat)
    plugin.Run(ctx)
    if ctx.Handled { t.Fatal("chat route must remain available to ordinary chat plugins") }
}

func TestRouteParserRejectsModelProse(t *testing.T) {
    if _, err := parseImageIntentRoute("我认为这是图片请求"); err == nil {
        t.Fatal("router must accept only chat or image")
    }
}
```

- [ ] **Step 2: Run tests to verify failure.**

Run: `go test ./plugin/plugins ./startup -run 'ImageSession|PluginRegistered' -count=1`

Expected: FAIL until plugin and registration are present.

- [ ] **Step 3: Implement image-message and quoted-image handling.**

For a raw image message, call `ImageSessionService.AddAsset` with role `target` when it is the first asset and `reference` afterward, then attempt quote-number sending. For a quoted image text message, bind the quoted image as the explicit target and immediately call `DirectImageService.Generate`. For a new text-only request, call `ImageIntentRouter.Route` once with a system instruction that permits only `chat` or `image`, disables tools, and supplies only the current prompt. Parse exact output; on classifier error fall back to `chat` so an ambiguous failure cannot trigger image generation. Never use the classifier response as the image prompt.

- [ ] **Step 4: Implement active-session text handling and controls.**

When a session is `idle`, pass all non-control text exactly as the prompt. Resolve only explicit asset references present in the user's text and submit those message IDs. Implement `图N设为目标图`, `删除图N`, `查看素材`, and `结束作图` as metadata operations. Control replies must not call the image model.

- [ ] **Step 5: Send outputs and update target.**

On successful `DirectImageService.Generate`, call `SendImageMessageByLocalPathWithResult`, add the returned persisted message ID as the new target/output asset, and refresh TTL. Keep `SendImageMessageByLocalPath` as a wrapper that discards the result so existing callers do not break. On failure send a concise real error and keep the prior target.

Emit one structured `[ImageTask]` completion log containing `request_id`, `session_id`, `raw_prompt`, `sanitized_prompt`, target/reference message IDs, model, requested size, returned sizes, elapsed duration and upstream error. Never log API keys or image binary data. Add a log-capture test that asserts `raw_prompt` and `sanitized_prompt` remain distinguishable and that the requested/returned sizes are present.

- [ ] **Step 6: Register before chat plugins and run focused tests.**

Move `ImageAutoUploadPlugin` ahead of `ImageSessionPlugin` so automatic OSS upload still runs for raw image messages. Register `ImageSessionPlugin` after auto-upload but before `ChatRoomAIChatPlugin` and `FriendAIChatPlugin` process text. Run:

```bash
gofmt -w plugin/plugins/image_session.go plugin/plugins/image_session_test.go startup/plugin.go
go test ./plugin/plugins ./startup -run 'ImageSession|PluginRegistered' -count=1
```

Expected: PASS; ordinary chat tests remain unchanged.

- [ ] **Step 7: Commit the routing unit.**

```bash
git add plugin/plugins/image_session.go plugin/plugins/image_session_test.go service/image_intent_router.go service/image_intent_router_test.go service/message.go startup/plugin.go interface/plugin/message.go
git commit -m "feat: route active image sessions directly"
```

### Task 4: Add Quote Replies and Four-Minute Warning Worker

**Files:**
- Create: `service/quote_message.go`
- Test: `service/quote_message_test.go`
- Create: `service/image_session_expiry.go`
- Test: `service/image_session_expiry_test.go`
- Create: `common_cron/image_session_expiry.go`
- Modify: `common_cron/manage.go`
- Modify: `vars/cron.go`

**Interfaces:**
- `func BuildQuoteAppXML(refer *model.Message, title string) (string, error)`
- `func (s *MessageService) SendQuotedText(toWxID string, refer *model.Message, title string, atWxID ...string) error`
- `func NewImageSessionExpiryWorker(ctx context.Context) *ImageSessionExpiryWorker`
- `func (w *ImageSessionExpiryWorker) ScanOnce(now time.Time) error`

- [ ] **Step 1: Add failing XML and warning tests.**

```go
func TestBuildQuoteAppXMLEscapesTitleAndIncludesReferMessage(t *testing.T) {
    refer := &model.Message{MsgId: 123, FromWxID: "room@chatroom", SenderWxID: "wxid_a", Content: "<img/>"}
    got, err := BuildQuoteAppXML(refer, "图1 · 目标图 & 已加入")
    if err != nil { t.Fatal(err) }
    for _, want := range []string{"<type>57</type>", "<svrid>123</svrid>", "图1 · 目标图 &amp; 已加入"} {
        if !strings.Contains(got, want) { t.Fatalf("XML missing %q: %s", want, got) }
    }
}

func TestExpiryScanSkipsProcessingSession(t *testing.T) {
    worker, store, sender := newExpiryWorkerTest(t)
    store.Put(sessionAt("processing", time.Now().Add(-4*time.Minute)))
    if err := worker.ScanOnce(time.Now()); err != nil { t.Fatal(err) }
    if sender.Count() != 0 { t.Fatalf("warnings = %d", sender.Count()) }
}
```

- [ ] **Step 2: Run focused tests and verify failure.**

Run: `go test ./service -run 'Quote|Expiry' -count=1`

Expected: FAIL until the XML builder and worker exist.

- [ ] **Step 3: Implement Type 57 XML and fallback.**

Use `encoding/xml` or an explicit XML struct, escape all user-controlled title/content values, and include the referenced message's server ID, sender, chat user, display name and content. Call existing `SendAppMessage` first. If the protocol returns an error, send ordinary text with the same number/role and record the downgrade.

- [ ] **Step 4: Implement persistent warning scanning.**

Scan Redis session keys using a bounded cursor. For idle sessions whose `last_activity` is between four and five minutes old, atomically mark `warning_sent` and send the group/private warning. Skip `processing`; delete only sessions past five minutes that remain idle. On send failure, clear the warning marker so the next scan retries.

- [ ] **Step 5: Wire the worker into startup and verify no duplicate scheduler registration.**

Add `vars.ImageSessionExpiryCron`, create `common_cron.NewImageSessionExpiryCron`, and register it from `CronManager.Start`. Schedule it every 15 seconds with gocron's duration API rather than a daily cron expression. The job calls `ScanOnce(time.Now())`; `CronManager.Stop` cancels the shared context and stops future scans.

- [ ] **Step 6: Run tests and commit.**

```bash
gofmt -w service/quote_message.go service/quote_message_test.go service/image_session_expiry.go service/image_session_expiry_test.go
go test ./service -run 'Quote|Expiry' -count=1
git add service/quote_message.go service/quote_message_test.go service/image_session_expiry.go service/image_session_expiry_test.go common_cron/image_session_expiry.go common_cron/manage.go vars/cron.go
git commit -m "feat: warn and quote image session users"
```

### Task 5: Remove Legacy Image Restrictions and Document FNOS Rollout

**Files:**
- Modify: `.deploy/skills/install-text-to-image-gpt-edit.sh`
- Modify: `docs/wechat-bot-fnos-deploy.md`
- Test: `.deploy/skills/test_text_to_image_edit_transport.py`

- [ ] **Step 1: Add a regression check for legacy behavior.**

Extend the Python test to load the generated/installed transport module and assert that a `gpt-image-2` edit request uses the supplied target size, while a generation request omits size and quality. Assert that the edit prompt equals the user prompt exactly and that the transport returns its JSON result without calling the WeChat image-send endpoint. Fixed sizes may remain only in provider branches for older models that require them.

- [ ] **Step 2: Update the installer and deployment documentation.**

Document the required environment/configuration values, the JSON result contract, temporary-directory permissions, model/upstream capability caveats, quote-message FNOS test command, image-session TTL/warning behavior, rollback procedure, and the fact that Mac source changes do not alter the N100 deployment.

- [ ] **Step 3: Run deployment checks.**

Run:

```bash
python3 -m unittest .deploy/skills/test_text_to_image_edit_transport.py
git diff --check
```

Expected: PASS and no whitespace errors.

- [ ] **Step 4: Commit deployment changes.**

```bash
git add .deploy/skills/install-text-to-image-gpt-edit.sh docs/wechat-bot-fnos-deploy.md
git commit -m "docs: document transparent image session deployment"
```

### Task 6: End-to-End Verification and FNOS Acceptance

**Files:**
- Modify only if verification exposes a defect; preserve all unrelated user changes.

- [ ] **Step 1: Run focused Go tests for all new units.**

```bash
go test ./service ./plugin/plugins ./startup -run 'ImageSession|DirectImage|Quote|Expiry|PluginRegistered' -count=1
```

- [ ] **Step 2: Run existing regression tests.**

```bash
go test ./pkg/skills ./pkg/robot ./utils ./service ./plugin/plugins ./startup
```

Report external-service or pre-existing failures separately; do not hide them as feature failures.

- [ ] **Step 3: Build the Linux binary.**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
```

- [ ] **Step 4: Build the candidate FNOS image using the repository's deployment workflow.**

Follow section 5.4 of `docs/wechat-bot-fnos-deploy.md`: build `wechat-robot-client-custom.new` with the FNOS `.tools/go/bin/go`, validate and rename it, tag the currently running client image as `jiqiren/wechat-robot-client:rollback-before-transparent-image-session`, then build the runtime image with `Dockerfile.fnos-runtime`. Do not claim runtime success until that image is installed on the N100 host.

- [ ] **Step 5: Execute the FNOS acceptance matrix.**

Verify all of the following with a real private chat and group chat:

```text
画一只穿宇航服的柴犬，在火星表面，电影海报风格
发送 1920x1344 图片 + “帮我加水印 仅限云盛公司内部专用”
发送三张素材并使用图1/图2/图3的复杂参考说明
四分钟无操作时提醒只艾特创建者
提醒后继续操作会刷新五分钟
五分钟无操作后素材状态消失
模型任务超过五分钟时不被清理
```

- [ ] **Step 6: Record final evidence.**

Capture request/output size logs, raw-prompt equality, quote-message success or fallback, warning timestamps, cleanup results and any upstream limitation. Only then report the implementation as complete.
