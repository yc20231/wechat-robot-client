# 电商图片意图归一化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ambiguous e-commerce image requests ask one style question instead of inventing a white background, while preserving explicit user requirements.

**Architecture:** Keep the existing AI agent and image script. Strengthen the `text-to-image` Skill instructions with semantic classification and a one-question ambiguity rule, and extend the quoted-image edit detector so quoted image creation requests can enter the Skill without unnecessary vision recognition.

**Tech Stack:** Markdown Skill instructions, Go intent helper and unit tests.

## Global Constraints

- Do not change OSS download behavior or the 5-minute reference-image window.
- Do not change `reference_count` semantics or four-image editing.
- Do not add a new model or dependency.
- Explicit user background, copy, layout, and preservation constraints must be passed through unchanged.

### Task 1: Expand Quoted Image Creation Intent

**Files:**
- Modify: `plugin/plugins/quoted_image_recognition.go:10-25`
- Test: `plugin/plugins/quoted_image_recognition_test.go`

**Interfaces:**
- Consumes: the trimmed text after the AI trigger word is removed.
- Produces: `isImageEditRequest(question string) bool` returning true for quoted-image creation/editing requests such as “做一张产品主图”.

- [ ] **Step 1: Write failing tests**

Add cases asserting `isImageEditRequest` returns true for `做一张适用于这个产品的电商主图`, `参考这张图制作宣传图`, and existing `1:1 还原`; assert it remains false for `这张图片里有什么`.

- [ ] **Step 2: Run the focused test and verify it fails**

```bash
.tools/go/bin/go test ./plugin/plugins -run 'IsImageEditRequest' -count=1
```

Expected: the new creation-intent cases fail because the current keyword list does not include these phrases.

- [ ] **Step 3: Implement the minimal keyword expansion**

Add semantic creation terms such as `做一张`, `制作`, `电商主图`, `产品主图`, `商品主图`, `宣传图`, and `海报` to the existing helper. Keep image questions such as `这张图片里有什么` false.

- [ ] **Step 4: Run the focused test and verify it passes**

```bash
.tools/go/bin/go test ./plugin/plugins -run 'IsImageEditRequest' -count=1
```

Expected: PASS.

### Task 2: Normalize Ambiguous E-commerce Requests in the Skill

**Files:**
- Modify: `.deploy/skills/text-to-image-gpt-edit.patch:1-110`
- Modify: `docs/wechat-bot-fnos-deploy.md` in the text-to-image deployment instructions.

**Interfaces:**
- Consumes: natural-language image requests passed to the AI agent.
- Produces: a script call whose prompt contains the current user's visual constraints.

- [ ] **Step 1: Update Skill instructions**

Add rules that classify explicit white-background requests, explicit promotional requests, and ambiguous “e-commerce main image” requests. Execute all classes directly without asking a style question:

```text
直接按当前请求执行，不询问 A/B；未明确指定时由 Skill 根据图片和文字意图自然完成构图。
```

State that the Skill must not invent pure-white background or “no copy/icons” constraints.

- [ ] **Step 2: Add examples for varied wording**

Document examples using `电商主图`, `产品主图`, `商品宣传图`, `平台上架白底图`, and `促销海报`, including the expected direct execution or clarification behavior.

- [ ] **Step 3: Verify the patch and docs are consistent**

```bash
git diff --check
rg -n '纯白背景|白底商品图|电商宣传图|电商主图' .deploy/skills/text-to-image-gpt-edit.patch docs/wechat-bot-fnos-deploy.md
```

Expected: the clarification wording and no-invention rule appear in both deployment-facing sources where applicable.

### Task 3: Regression Verification

**Files:**
- Test: `plugin/plugins/quoted_image_recognition_test.go`

- [ ] **Step 1: Run focused Go tests**

```bash
.tools/go/bin/go test ./service ./plugin/plugins \
  -run 'DownloadOriginalImageWithFallback|IsImageEditRequest|ImageRecognition|QuotedImage' \
  -count=1
```

Expected: both packages pass.

- [ ] **Step 2: Build the FlyOS target**

```bash
GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  .tools/go/bin/go build \
  -trimpath \
  -ldflags='-s -w -X main.Version=ecommerce-image-intent-v1' \
  -o /tmp/wechat-robot-client-ecommerce-image-intent
```

Expected: exit code 0 and an `ELF 64-bit x86-64` executable.

### Task 4: Persist A/B Image Choices

**Files:**
- Create: `plugin/plugins/pending_ecommerce_image.go`
- Create: `plugin/plugins/pending_ecommerce_image_test.go`
- Modify: `plugin/plugins/ai_chat.go`

**Interfaces:**
- Consumes: an ambiguous quoted-image request followed within 5 minutes by an A/B message from the same sender and conversation.
- Produces: a restored image reference and explicit image-edit request passed to the existing AI Skill flow.

- [x] Store the quoted target image ID and original request in Redis with a 5-minute TTL.
- [x] Isolate keys by robot, conversation, and sender.
- [x] Remove the legacy A/B style-choice flow.
