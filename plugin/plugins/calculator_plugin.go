package plugins

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"

	"wechat-robot-client/interface/plugin"
	"wechat-robot-client/pkg/templates/calculatorresult"
	"wechat-robot-client/service"
)

// CalculatorPlugin 塑料袋计算器插件：
// 群里 @机器人 或私聊发送 “30+5*50/2丝 查克数”，按与网站计算器一致的公式计算并回图。
type CalculatorPlugin struct{}

func NewCalculatorPlugin() plugin.MessageHandler {
	return &CalculatorPlugin{}
}

func (p *CalculatorPlugin) GetName() string {
	return "Calculator"
}

func (p *CalculatorPlugin) GetLabels() []string {
	return []string{"text", "chat"}
}

func (p *CalculatorPlugin) Match(ctx *plugin.MessageContext) bool {
	if ctx == nil || ctx.Message == nil {
		return false
	}
	if ctx.ReferMessage != nil {
		// 不解析引用消息里的计算请求
		return false
	}
	if !parseCalculatorTrigger(ctx.MessageContent) {
		return false
	}
	// 群聊必须 @机器人，避免误触发
	if ctx.Message.IsChatRoom && !ctx.Message.IsAtMe {
		return false
	}
	return true
}

func (p *CalculatorPlugin) PreAction(ctx *plugin.MessageContext) bool {
	if ctx.Message.IsChatRoom {
		return NewChatRoomCommonPlugin().PreAction(ctx)
	}
	return true
}

func (p *CalculatorPlugin) PostAction(ctx *plugin.MessageContext) {}

func (p *CalculatorPlugin) Run(ctx *plugin.MessageContext) {
	if !p.PreAction(ctx) {
		return
	}
	request, err := ParseCalculatorRequest(ctx.MessageContent)
	if err != nil {
		ctx.Handled = true
		ctx.MessageService.SendTextMessage(ctx.Message.FromWxID, calculatorUsageText, ctx.Message.SenderWxID)
		return
	}
	result, err := request.Calculate()
	if err != nil {
		ctx.Handled = true
		ctx.MessageService.SendTextMessage(ctx.Message.FromWxID, err.Error(), ctx.Message.SenderWxID)
		return
	}
	htmlContent, err := renderCalculatorResultHTML(buildCalculatorResultTemplateData(request, result))
	if err != nil {
		log.Printf("[Calculator] 渲染计算结果模板失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		ctx.Handled = true
		ctx.MessageService.SendTextMessage(ctx.Message.FromWxID, "计算结果图片生成失败，请稍后再试", ctx.Message.SenderWxID)
		return
	}
	routeContext := ctx.Context
	if routeContext == nil {
		routeContext = context.Background()
	}
	pngBytes, err := service.CaptureHTMLScreenshot(routeContext, htmlContent)
	if err != nil {
		log.Printf("[Calculator] 计算结果截图失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		ctx.Handled = true
		ctx.MessageService.SendTextMessage(ctx.Message.FromWxID, "计算结果图片生成失败，请稍后再试", ctx.Message.SenderWxID)
		return
	}
	if _, err := ctx.MessageService.MsgUploadImg(ctx.Message.FromWxID, bytes.NewReader(pngBytes)); err != nil {
		log.Printf("[Calculator] 发送计算结果图片失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		return
	}
	ctx.Handled = true
}

type calculatorResultTemplateData struct {
	LogoDataURI   template.URL
	MaterialLabel string
	BagLabel      string
	SpecLine      string
	WeightValue   string
	SilkValue     string
}

func buildCalculatorResultTemplateData(request *CalculatorRequest, result *CalculatorResult) calculatorResultTemplateData {
	specLine := fmt.Sprintf("%s*%scm", formatCalculatorNumber(result.TotalWidth), formatCalculatorNumber(result.Length))
	if request.Fold > 0 {
		// 与计算器页一致：带折边时展示 宽+(折+折)*长
		fold := formatCalculatorNumber(request.Fold)
		specLine = fmt.Sprintf("%s+(%s+%s)*%scm", formatCalculatorNumber(request.Width), fold, fold, formatCalculatorNumber(result.Length))
	}
	return calculatorResultTemplateData{
		MaterialLabel: result.Formula.ShortName,
		BagLabel:      result.Formula.Bag,
		SpecLine:      specLine,
		WeightValue:   formatCalculatorNumber(result.SingleWeight),
		SilkValue:     formatCalculatorNumber(result.Silk),
	}
}

func renderCalculatorResultHTML(data calculatorResultTemplateData) (string, error) {
	logoBytes, err := calculatorresult.FS.ReadFile("logo.png")
	if err != nil {
		return "", err
	}
	// html/template 默认拦截 data: 协议，需用 template.URL 显式标记可信
	data.LogoDataURI = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(logoBytes))
	templateBytes, err := calculatorresult.FS.ReadFile("calculator_result.html")
	if err != nil {
		return "", err
	}
	tpl, err := template.New("calculator_result.html").Parse(string(templateBytes))
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	if err := tpl.Execute(&buffer, data); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
