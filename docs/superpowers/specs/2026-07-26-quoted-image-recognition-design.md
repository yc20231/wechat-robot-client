# Quoted Image Recognition Design

## Goal

Make quoted-image questions use the configured image recognition model as a
real multimodal request before the existing chat agent answers. Exa remains a
web search tool and must not be used to fetch direct image URLs.

## Current Problem

The quoted-image path already detects a real WeChat mention, resolves the
referenced image message, and uploads the image to OSS. It then adds the OSS URL
to the chat context as plain text:

```text
<user question>

图片地址: https://example.com/image.jpg
```

Because the chat request contains no `image_url` content part, the model cannot
inspect the image pixels. It may call `exa__web_fetch_exa`, which rejects direct
JPEG content with `CRAWL_UNEXPECTED_CONTENT_TYPE`. The configured
`ImageRecognitionModel` is loaded into `AIConfig` but is not used by the chat
path.

## Scope

This change covers a user quoting a normal WeChat image and asking the robot a
question in the same quoted message. It preserves the existing OSS upload,
chat context, tools, memory, and quoted-image editing flows.

The initial change does not add image recognition for standalone images,
emoticons, videos, files, or multiple quoted images. It does not replace Exa
for web search and does not add a third-party vision MCP server.

## Architecture

Add a focused image recognition service in the service layer. It uses the
effective AI configuration selected by the existing global and per-chat-room
settings logic:

- Base URL: `AIConfig.BaseURL`
- API key: `AIConfig.APIKey`
- model: `AIConfig.ImageRecognitionModel`

The service sends a Chat Completions request directly to the configured image
model. The user message contains both a text instruction derived from the
quoted question and a standard `image_url` content part pointing at the OSS
URL. This recognition request does not receive Agent tools, MCP tools, Skills,
memory, or group context.

The existing main chat request remains unchanged structurally and continues to
use `AIConfig.Model` through `AgentService`. Before that request, the quoted
message is enriched with the image recognition result.

## Data Flow

1. Receive a quoted app message (`app_msg_type=57`) with a valid mention.
2. Resolve the referenced server message ID to the stored image message.
3. Reuse its `AttachmentUrl`, or upload the image to OSS when the URL is empty.
4. Call the configured image recognition model with:
   - the user's quoted-message question;
   - the OSS URL as an `image_url` part.
5. On success, replace the current quoted-image chat entry with text containing:
   - the original user question;
   - the image recognition result;
   - the original image URL for existing image-editing workflows;
   - an instruction that the image is already analyzed and direct image URLs
     must not be fetched with web crawling tools.
6. Send the enriched conversation to the existing main chat model and Agent.
7. Send the main model's final response through the existing WeChat reply path.

The referenced message ID remains in `RobotContext.RefMessageID`, so existing
image editing Skills can still locate and download the original image.

## Failure Handling

Image recognition failures do not stop the main chat request. They are logged
with enough context to identify the model and referenced message without
logging API keys.

The current quoted-image chat entry is replaced with text containing:

- the original question;
- a concise statement that image recognition failed;
- the image URL for existing editing flows;
- explicit instructions not to guess image contents and not to call Exa or
  another web crawler for the direct image URL.

An empty image recognition model, empty image URL, transport failure, provider
error, or empty model response all follow this fallback. The user receives the
main model's response rather than a second standalone error message.

## Compatibility

The configured provider and `ImageRecognitionModel` must support OpenAI-style
Chat Completions with multimodal `image_url` input. The main chat model does not
need native vision support because it receives text produced by the image
recognition model.

No database migration or API schema change is required. The existing
`image_recognition_model` setting becomes functional without changing the
management frontend contract.

## Testing

Add focused tests for these behaviors:

1. Image recognition parameters use `ImageRecognitionModel`, include the user
   question as text, and include the OSS URL as a real `image_url` part.
2. A successful recognition result produces an enriched main-chat message with
   the original question, recognition result, and crawler prohibition.
3. A recognition failure still produces a main-chat message, preserves the
   original question and image URL, and explicitly prohibits guessing and web
   crawling of the image.
4. Non-image quoted messages retain their existing behavior.

Tests should exercise pure request/message construction helpers where possible.
Provider transport can be covered with an `httptest` OpenAI-compatible endpoint
if needed, without requiring external AI credentials.

## Non-Goals

- Replacing Exa for normal web search.
- Installing or operating a vision MCP server.
- Adding separate image-model API credentials or a separate base URL.
- Changing the existing image generation/editing Skill.
- Automatically identifying vehicles through a domain-specific vehicle API.
