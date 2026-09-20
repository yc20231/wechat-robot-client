package service

import "context"

const defaultHTMLScreenshotWidth = 960

// CaptureHTMLScreenshot 把 HTML 渲染成整页 PNG 截图，供消息插件生成结果图片。
func CaptureHTMLScreenshot(ctx context.Context, htmlContent string) ([]byte, error) {
	return captureHTMLScreenshot(ctx, htmlContent, defaultHTMLScreenshotWidth)
}

// CaptureHTMLScreenshotWidth 按指定宽度截图；宽度 <= 0 时回退到默认 960。
func CaptureHTMLScreenshotWidth(ctx context.Context, htmlContent string, width int) ([]byte, error) {
	if width <= 0 {
		width = defaultHTMLScreenshotWidth
	}
	return captureHTMLScreenshot(ctx, htmlContent, width)
}
