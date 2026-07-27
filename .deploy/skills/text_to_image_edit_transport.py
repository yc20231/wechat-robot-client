from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
import uuid


_PACKY_FILE_ERROR = "does not accept multipart file upload"
_PACKY_URL_HINT = "image_url"


class AttrDict(dict):
    __getattr__ = dict.get


def _as_attr_dict(value):
    if isinstance(value, dict):
        return AttrDict({key: _as_attr_dict(item) for key, item in value.items()})
    if isinstance(value, list):
        return [_as_attr_dict(item) for item in value]
    return value


def should_retry_with_image_url(mode: str, error: Exception, image_count: int) -> bool:
    if str(mode or "auto").strip().lower() != "auto" or image_count != 1:
        return False
    message = str(error).lower()
    return _PACKY_FILE_ERROR in message and _PACKY_URL_HINT in message


def _multipart_body(fields: list[tuple[str, str]]) -> tuple[bytes, str]:
    boundary = f"wechat-robot-{uuid.uuid4().hex}"
    chunks: list[bytes] = []
    for name, value in fields:
        chunks.extend(
            [
                f"--{boundary}\r\n".encode("ascii"),
                f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("ascii"),
                str(value).encode("utf-8"),
                b"\r\n",
            ]
        )
    chunks.append(f"--{boundary}--\r\n".encode("ascii"))
    return b"".join(chunks), boundary


def post_image_url_edit(
    config: dict,
    *,
    model: str,
    prompt: str,
    image_urls: list[str],
    n: int,
    size: str,
    quality: str,
) -> dict:
    if len(image_urls) != 1:
        raise RuntimeError("当前中转站的 image_url 模式尚不支持多图编辑")

    image_url = str(image_urls[0] or "").strip()
    if urllib.parse.urlparse(image_url).scheme not in {"http", "https"}:
        raise RuntimeError("图片编辑缺少可公网访问的 image_url")

    api_key = str(config.get("api_key", "") or "").strip()
    if not api_key:
        raise RuntimeError("OpenAI 绘图配置缺少 api_key")
    base_url = str(config.get("base_url", "") or "https://api.openai.com/v1").rstrip("/")
    body, boundary = _multipart_body(
        [
            ("model", model or "gpt-image-2"),
            ("prompt", prompt),
            ("image_url", image_url),
            ("n", str(n)),
            ("size", size),
            ("quality", quality),
            ("response_format", "url"),
        ]
    )
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": f"multipart/form-data; boundary={boundary}",
        "User-Agent": "wechat-robot/text-to-image",
    }
    request = urllib.request.Request(
        f"{base_url}/images/edits",
        data=body,
        headers=headers,
        method="POST",
    )
    timeout = float(config.get("timeout") or 300)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return _as_attr_dict(json.loads(response.read().decode("utf-8")))
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"image_url 图片编辑失败: HTTP {error.code} {detail}") from error
