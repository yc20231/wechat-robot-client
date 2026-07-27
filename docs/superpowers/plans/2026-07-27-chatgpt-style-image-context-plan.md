# ChatGPT-Style Image Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Prevent old image intent from leaking into new tasks while keeping explicit references and natural image questions reliable.

**Architecture:** Keep image routing in `quoted_image_recognition.go`, add separate predicates for image questions and image tasks, and make `AIChatPlugin.Run` isolate context for all new image tasks unless continuation language is explicit. Recent-image lookup remains scoped to the same conversation and sender.

**Tech Stack:** Go, Go test, GORM, OpenAI chat message parameters.

## Global Constraints

- Explicit WeChat quotes are authoritative and are not limited by the recent-image window.
- Unquoted automatic binding only uses the same sender's latest image within five minutes.
- New image tasks are isolated unless continuation is explicit.

### Task 1: Add routing regression tests

**Files:**
- Modify: `plugin/plugins/quoted_image_recognition_test.go`

- [ ] Add tests proving `参考图里有什么` and `参考这张图分析一下` are image questions, while `参考这张图做一张海报` remains an image task.
- [ ] Add tests proving `画一只猫` and `生成一张海报` are new image tasks and `继续刚才的图` is continuation.
- [ ] Run the focused tests and confirm they fail against the current predicates.

### Task 2: Separate image questions from image tasks

**Files:**
- Modify: `plugin/plugins/quoted_image_recognition.go`
- Test: `plugin/plugins/quoted_image_recognition_test.go`

- [ ] Remove broad standalone `参考`/`基于` edit matching.
- [ ] Add a predicate that identifies explicit image questions and excludes them from edit routing.
- [ ] Keep concrete generation/edit verbs and explicit reference-plus-action phrases classified as image tasks.
- [ ] Run the routing tests and confirm they pass.

### Task 3: Isolate all new image task contexts

**Files:**
- Modify: `plugin/plugins/ai_chat.go`
- Test: `plugin/plugins/quoted_image_recognition_test.go`

- [ ] Add a shared predicate check in `Run` so no-reference image generation also receives only the current request.
- [ ] Keep explicit continuation requests on the existing AI context path.
- [ ] Preserve reference-message IDs for the image skill and keep image recognition for question requests.
- [ ] Run focused plugin tests and the Linux amd64 build.

### Task 4: Final verification

**Files:**
- No production files.

- [ ] Run `git diff --check`.
- [ ] Run targeted plugin/service tests.
- [ ] Record unrelated full-suite failures without changing them.
