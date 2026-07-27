# ChatGPT-Style Image Context Design

## Goal

Make image generation and editing behave like ChatGPT.com: each new image task follows the current request directly, while explicit continuation can reuse the prior image task.

## Behavior

- An explicit WeChat quote is authoritative and is not limited by the recent-image window.
- Without a quote, phrases such as `参考这张图` or `刚发的图` may bind the latest image sent by the same sender in the same conversation within five minutes.
- Automatic binding never selects another group member's image.
- Image questions such as `这张图里有什么` or `参考图分析一下` use image recognition and do not enter image editing.
- Image creation/editing phrases such as `参考这张图做海报` or `基于这张图换背景` enter the image skill.
- New image creation/editing tasks use only the current request as model context, regardless of whether a reference image is present.
- Only explicit continuation language such as `继续`, `接着`, `沿用上一张`, or `在刚才结果上修改` reuses prior image context.

## Implementation

- Separate image-task detection from image-question detection in `plugin/plugins/quoted_image_recognition.go`.
- Keep recent-image lookup scoped by conversation, sender, image type, message order, and five-minute window.
- In `plugin/plugins/ai_chat.go`, isolate the model message list for all new image tasks, not only quoted image edits.
- Preserve the existing reference-message ID passed to the image skill so explicit quotes and automatic bindings continue to work.

## Verification

- Add regression tests for reference-image questions not being classified as edits.
- Add tests for no-reference image tasks being isolated from prior context.
- Keep existing tests for explicit quotes, recent-image binding, continuation, and multi-image editing.
- Run targeted Go tests and a Linux amd64 build; report unrelated full-suite failures separately.
