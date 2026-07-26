# Direct Image Recognition And Four-Image Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Avoid forced OSS upload for quoted-image recognition and extend the local GPT Image editing workflow from one target plus one reference to one target plus up to three references.

**Architecture:** Quoted-image Q&A will download the referenced message through the existing `AttachDownloadService`/`RobotRuntime` path, encode the bytes as an inline data URL, and send that data URL to the configured vision model. The text-to-image Skill patch will resolve a configurable count of recent reference messages within the existing five-minute/same-conversation/same-sender window, keep the quoted image first, and send all selected files to the multipart image edit request.

**Tech Stack:** Go, GORM repositories, OpenAI-compatible Chat Completions, Python Skill runner, multipart `images.edit`, existing FNOS Docker runtime.

## Global Constraints

- A quoted target image is not subject to the five-minute reference window; it remains valid while its message and image can be downloaded.
- Unquoted reference images are limited to the same conversation, same sender, and five minutes before the trigger message.
- The supported local editing shape is one target plus at most three references (four input images total).
- Text-to-image generation, single-image editing, robot-output continuation, and output OSS persistence keep their existing user-facing behavior.
- Image recognition input is held in memory with the existing 25 MB limit and is not persisted as a local file.

### Task 1: Direct quoted-image recognition download

**Files:**
- Modify: `service/ai_image_recognition.go`
- Modify: `plugin/plugins/ai_chat.go`
- Test: `service/ai_image_recognition_test.go`
- Test: `plugin/plugins/quoted_image_recognition_test.go`

**Interfaces:**
- Add a message-oriented recognition entry point that accepts a message ID and uses `AttachDownloadService.DownloadImage`.
- Keep the existing URL-oriented helper for compatibility and unit testing.
- The AI chat plugin must no longer call `AIImageUploadPlugin` for quoted images before recognition.

- [ ] Add a failing test proving message bytes become a `data:image/...;base64,...` payload without an OSS URL.
- [ ] Run the focused Go tests and observe the failure.
- [ ] Implement message download, MIME detection, base64 encoding, and recognition invocation.
- [ ] Remove the quoted-image pre-upload dependency while preserving context marking and non-image upload behavior.
- [ ] Run `go test ./service ./plugin/plugins` with the focused recognition tests.

### Task 2: Resolve up to three recent references

**Files:**
- Modify: `.deploy/skills/text-to-image-gpt-edit.patch`
- Modify: `docs/wechat-bot-mcp-config.md`
- Modify: `docs/wechat-bot-fnos-deploy.md`

**Interfaces:**
- Replace the single-reference resolution with `reference_count` in the Skill parameters, accepting `0..3`; keep `--use-latest-reference` as a compatibility alias for count `1`.
- Add `_find_recent_reference_images(...) -> list[int]`, querying at most the requested count and reversing results to chronological order.
- Pass `[target, reference1, reference2, reference3]` to `call_openai_edit` and describe the role of each reference in the edit prompt.

- [ ] Add/adjust local script checks for count validation, target exclusion, five-minute filtering, and chronological ordering.
- [ ] Apply the patch to a clean upstream Skill staging directory and run `python3 -m py_compile`.
- [ ] Update the usage documentation with one-target-plus-three-reference examples and the old two-image example.

### Task 3: Verification and FNOS handoff

**Files:**
- No source changes unless verification exposes a defect.

- [ ] Run `go test ./...`.
- [ ] Build Linux amd64 with the repository's `.tools/go/bin/go` toolchain and `CGO_ENABLED=0`.
- [ ] Verify the generated binary and runtime Docker image are executable.
- [ ] Provide the FNOS commands to replace only the client container image; do not rebuild the server container or rescan WeChat.

