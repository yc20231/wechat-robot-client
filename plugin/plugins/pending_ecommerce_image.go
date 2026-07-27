package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"wechat-robot-client/interface/plugin"
	"wechat-robot-client/model"
	"wechat-robot-client/vars"
)

const pendingEcommerceImageTTL = 5 * time.Minute

type pendingEcommerceImageRequest struct {
	RefMessageID     int64  `json:"ref_message_id"`
	RefMessageMsgID  int64  `json:"ref_message_msg_id"`
	RefAttachmentURL string `json:"ref_attachment_url,omitempty"`
	OriginalRequest  string `json:"original_request"`
}

func pendingEcommerceImageKey(robotWxID string, message *model.Message) string {
	if message == nil {
		return ""
	}
	requesterWxID := strings.TrimSpace(message.SenderWxID)
	if requesterWxID == "" {
		requesterWxID = strings.TrimSpace(message.FromWxID)
	}
	return fmt.Sprintf(
		"wechat-robot:%s:pending-ecommerce-image:%s:%s",
		strings.TrimSpace(robotWxID),
		strings.TrimSpace(message.FromWxID),
		requesterWxID,
	)
}

func savePendingEcommerceImageRequest(ctx context.Context, message *model.Message, request pendingEcommerceImageRequest) error {
	if vars.RedisClient == nil {
		return errors.New("Redis 未初始化")
	}
	if request.RefMessageID <= 0 || strings.TrimSpace(request.OriginalRequest) == "" {
		return errors.New("待处理电商图片请求无效")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("序列化待处理电商图片请求失败: %w", err)
	}
	return vars.RedisClient.Set(
		ctx,
		pendingEcommerceImageKey(vars.RobotRuntime.WxID, message),
		payload,
		pendingEcommerceImageTTL,
	).Err()
}

func takePendingEcommerceImageRequest(ctx context.Context, message *model.Message) (*pendingEcommerceImageRequest, bool, error) {
	if vars.RedisClient == nil {
		return nil, false, errors.New("Redis 未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := pendingEcommerceImageKey(vars.RobotRuntime.WxID, message)
	payload, err := vars.RedisClient.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取待处理电商图片请求失败: %w", err)
	}

	var request pendingEcommerceImageRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, false, fmt.Errorf("解析待处理电商图片请求失败: %w", err)
	}
	if request.RefMessageID <= 0 || strings.TrimSpace(request.OriginalRequest) == "" {
		return nil, false, errors.New("待处理电商图片请求无效")
	}
	if err := vars.RedisClient.Del(ctx, key).Err(); err != nil {
		return &request, true, fmt.Errorf("删除待处理电商图片请求失败: %w", err)
	}
	return &request, true, nil
}

func applyPendingEcommerceChoice(ctx *plugin.MessageContext, request *pendingEcommerceImageRequest, style string) {
	if ctx == nil || ctx.Message == nil || request == nil {
		return
	}
	resolvedRequest := buildEcommerceStyleRequest(request.OriginalRequest, style)
	ctx.ReferMessage = &model.Message{
		ID:            request.RefMessageID,
		MsgId:         request.RefMessageMsgID,
		Type:          model.MsgTypeImage,
		AttachmentUrl: request.RefAttachmentURL,
	}
	ctx.MessageContent = resolvedRequest
	ctx.Message.Content = resolvedRequest
}

func pendingEcommerceRequestFromReference(refMessage *model.Message, originalRequest string) *pendingEcommerceImageRequest {
	if refMessage == nil || refMessage.Type != model.MsgTypeImage {
		return nil
	}
	return &pendingEcommerceImageRequest{
		RefMessageID:     refMessage.ID,
		RefMessageMsgID:  refMessage.MsgId,
		RefAttachmentURL: refMessage.AttachmentUrl,
		OriginalRequest:  strings.TrimSpace(originalRequest),
	}
}

func (p *AIChatPlugin) prepareEcommerceStyleFlow(ctx *plugin.MessageContext, aiTriggerWord string) bool {
	if ctx == nil || ctx.Message == nil {
		return false
	}
	question := p.trimAITriggerFromText(ctx.MessageContent, aiTriggerWord)

	if style, ok := parseEcommerceStyleChoice(question); ok {
		pending, found, err := takePendingEcommerceImageRequest(ctx.Context, ctx.Message)
		if err != nil {
			log.Printf("[EcommerceImage] 读取待选择任务异常: from=%s sender=%s err=%v", ctx.Message.FromWxID, ctx.Message.SenderWxID, err)
		}
		if ctx.ReferMessage != nil && ctx.ReferMessage.Type == model.MsgTypeImage {
			if !found || pending == nil {
				pending = pendingEcommerceRequestFromReference(ctx.ReferMessage, "参考引用图片制作电商主图")
			}
			pending.RefMessageID = ctx.ReferMessage.ID
			pending.RefMessageMsgID = ctx.ReferMessage.MsgId
			pending.RefAttachmentURL = ctx.ReferMessage.AttachmentUrl
			found = true
		}
		if !found || pending == nil {
			p.SendMessage(ctx, "没有找到待处理的电商主图任务，可能已经超过 5 分钟。请重新引用产品图片并发送完整要求。")
			ctx.Handled = true
			return true
		}
		applyPendingEcommerceChoice(ctx, pending, style)
		log.Printf("[EcommerceImage] 已恢复待选择任务: style=%s ref_msg_id=%d from=%s sender=%s", style, pending.RefMessageMsgID, ctx.Message.FromWxID, ctx.Message.SenderWxID)
		return false
	}

	if ctx.ReferMessage == nil || ctx.ReferMessage.Type != model.MsgTypeImage || !isAmbiguousEcommerceImageRequest(question) {
		return false
	}
	pending := pendingEcommerceRequestFromReference(ctx.ReferMessage, question)
	if err := savePendingEcommerceImageRequest(ctx.Context, ctx.Message, *pending); err != nil {
		log.Printf("[EcommerceImage] 保存待选择任务失败: ref_msg_id=%d from=%s sender=%s err=%v", ctx.ReferMessage.MsgId, ctx.Message.FromWxID, ctx.Message.SenderWxID, err)
		p.SendMessage(ctx, "暂时无法保存这次图片选择，请重新引用图片，并明确说明需要白底商品图或有背景和卖点文案的宣传图。")
	} else {
		log.Printf("[EcommerceImage] 已保存待选择任务: ref_msg_id=%d from=%s sender=%s ttl=%v", ctx.ReferMessage.MsgId, ctx.Message.FromWxID, ctx.Message.SenderWxID, pendingEcommerceImageTTL)
		p.SendMessage(ctx, ecommerceStyleQuestion)
	}
	ctx.Handled = true
	return true
}
