package safetyreminder

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	templateassets "wechat-robot-client/pkg/templates/safetyreminder"
)

const (
	PosterWidth              = 1279
	PosterHeight             = 1706
	posterBackgroundVariants = 5
)

type posterTemplateData struct {
	Background string
	Date       string
	Focus      string
	Points     [3]string
	Slogan     string
}

var chineseWeekdays = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

func Render(ctx context.Context, content PosterContent) ([]byte, error) {
	htmlContent, err := renderHTML(content)
	if err != nil {
		return nil, err
	}
	return capturePoster(ctx, htmlContent)
}

func renderHTML(content PosterContent) (string, error) {
	templateBytes, err := templateassets.Assets.ReadFile("poster.html")
	if err != nil {
		return "", fmt.Errorf("读取安全提醒模板失败: %w", err)
	}
	backgroundBytes, err := templateassets.Assets.ReadFile(backgroundAssetForDate(content.Date))
	if err != nil {
		return "", fmt.Errorf("读取安全提醒背景失败: %w", err)
	}
	tpl, err := htmltemplate.New("poster.html").Parse(string(templateBytes))
	if err != nil {
		return "", fmt.Errorf("解析安全提醒模板失败: %w", err)
	}

	data := posterTemplateData{
		Background: base64.StdEncoding.EncodeToString(backgroundBytes),
		Date:       fmt.Sprintf("%d.%02d.%02d %s", content.Date.Year(), content.Date.Month(), content.Date.Day(), chineseWeekdays[content.Date.Weekday()]),
		Focus:      content.Focus,
		Points:     content.Points,
		Slogan:     content.Slogan,
	}
	var output bytes.Buffer
	if err := tpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("生成安全提醒页面失败: %w", err)
	}
	return output.String(), nil
}

func backgroundAssetForDate(date time.Time) string {
	variant := cycleIndexForDate(date, posterBackgroundVariants) + 1
	if variant == 1 {
		return "assets/poster-background.png"
	}
	return fmt.Sprintf("assets/poster-background-%d.png", variant)
}

func capturePoster(ctx context.Context, htmlContent string) ([]byte, error) {
	tempFile, err := os.CreateTemp("", "safety_reminder_*.html")
	if err != nil {
		return nil, fmt.Errorf("创建安全提醒临时页面失败: %w", err)
	}
	tempFileName := tempFile.Name()
	defer os.Remove(tempFileName)
	if _, err := tempFile.WriteString(htmlContent); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("写入安全提醒临时页面失败: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("关闭安全提醒临时页面失败: %w", err)
	}

	allocatorOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WindowSize(PosterWidth, PosterHeight),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer allocatorCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	defer browserCancel()
	timeoutCtx, timeoutCancel := context.WithTimeout(browserCtx, 45*time.Second)
	defer timeoutCancel()

	fileURL := url.URL{Scheme: "file", Path: tempFileName}
	var pngBytes []byte
	if err := chromedp.Run(timeoutCtx,
		chromedp.EmulateViewport(PosterWidth, PosterHeight),
		chromedp.Navigate(fileURL.String()),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(`window.posterReady`, nil),
		chromedp.CaptureScreenshot(&pngBytes),
	); err != nil {
		return nil, fmt.Errorf("截取安全提醒图片失败: %w", err)
	}
	return pngBytes, nil
}
