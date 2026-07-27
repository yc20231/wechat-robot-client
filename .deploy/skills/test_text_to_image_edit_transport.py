import importlib.util
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("text_to_image_edit_transport.py")
SPEC = importlib.util.spec_from_file_location("text_to_image_edit_transport", MODULE_PATH)
transport = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(transport)


class _CaptureHandler(BaseHTTPRequestHandler):
    request_path = ""
    request_headers = None
    request_body = b""

    def do_POST(self):
        type(self).request_path = self.path
        type(self).request_headers = self.headers
        type(self).request_body = self.rfile.read(int(self.headers["Content-Length"]))
        payload = json.dumps({"data": [{"url": "https://cdn.example.com/result.png"}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


class ImageURLTransportTest(unittest.TestCase):
    def test_posts_public_url_as_a_text_multipart_field(self):
        server = ThreadingHTTPServer(("127.0.0.1", 0), _CaptureHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)

        response = transport.post_image_url_edit(
            {
                "base_url": f"http://127.0.0.1:{server.server_port}/v1",
                "api_key": "secret-key",
                "timeout": 5,
            },
            model="gpt-image-2",
            prompt="make the suit blue and white",
            image_urls=["https://images.example.com/input.png"],
            n=1,
            size="1024x1536",
            quality="high",
        )

        self.assertEqual(response["data"][0]["url"], "https://cdn.example.com/result.png")
        self.assertEqual(_CaptureHandler.request_path, "/v1/images/edits")
        self.assertEqual(_CaptureHandler.request_headers["Authorization"], "Bearer secret-key")
        self.assertIn("multipart/form-data; boundary=", _CaptureHandler.request_headers["Content-Type"])
        body = _CaptureHandler.request_body
        self.assertIn(b'name="image_url"\r\n\r\nhttps://images.example.com/input.png', body)
        self.assertNotIn(b'filename=', body)

    def test_auto_retry_requires_the_packy_conversion_error(self):
        packy_error = RuntimeError(
            "gpt-image-2 does not accept multipart file upload; "
            "please provide a public URL via 'image_url' form field instead"
        )
        self.assertTrue(transport.should_retry_with_image_url("auto", packy_error, 1))
        self.assertFalse(transport.should_retry_with_image_url("file", packy_error, 1))
        self.assertFalse(
            transport.should_retry_with_image_url(
                "auto", RuntimeError("Billing hard limit has been reached"), 1
            )
        )

    def test_url_transport_rejects_undocumented_multiple_images(self):
        with self.assertRaisesRegex(RuntimeError, "多图"):
            transport.post_image_url_edit(
                {"base_url": "https://api.example.com/v1", "api_key": "key"},
                model="gpt-image-2",
                prompt="combine",
                image_urls=["https://example.com/target.png", "https://example.com/ref.png"],
                n=1,
                size="1024x1024",
                quality="high",
            )


if __name__ == "__main__":
    unittest.main()
