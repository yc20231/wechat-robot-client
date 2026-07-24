package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"wechat-robot-client/pkg/safetyreminder"
	"wechat-robot-client/vars"
)

var ErrSafetyReminderAlreadySent = errors.New("今日安全提醒已发送")

type SafetyReminderService struct {
	ctx context.Context
}

type SafetyReminderSendResult struct {
	Date             string `json:"date"`
	Focus            string `json:"focus"`
	TargetChatRoomID string `json:"target_chat_room_id"`
}

func NewSafetyReminderService(ctx context.Context) *SafetyReminderService {
	return &SafetyReminderService{ctx: ctx}
}

func (s *SafetyReminderService) Preview(date time.Time, topicsFile string) ([]byte, safetyreminder.PosterContent, error) {
	topics, err := safetyreminder.LoadTopics(topicsFile)
	if err != nil {
		return nil, safetyreminder.PosterContent{}, err
	}
	content, err := safetyreminder.ContentForDate(date, topics)
	if err != nil {
		return nil, safetyreminder.PosterContent{}, err
	}
	pngBytes, err := safetyreminder.Render(s.ctx, content)
	if err != nil {
		return nil, safetyreminder.PosterContent{}, err
	}
	return pngBytes, content, nil
}

// Send renders and uploads one poster. deduplicate must be true for scheduled sends.
func (s *SafetyReminderService) Send(date time.Time, config safetyreminder.Config, deduplicate bool) (SafetyReminderSendResult, error) {
	if err := config.ValidateForSend(); err != nil {
		return SafetyReminderSendResult{}, err
	}

	dateValue := date.Format("2006-01-02")
	dedupeKey := fmt.Sprintf("safety-reminder:sent:%s:%s", dateValue, config.TargetChatRoomID)
	if deduplicate {
		if vars.RedisClient == nil {
			return SafetyReminderSendResult{}, errors.New("Redis 未初始化，无法保证安全提醒不重复发送")
		}
		acquired, err := vars.RedisClient.SetNX(s.ctx, dedupeKey, "sending", 48*time.Hour).Result()
		if err != nil {
			return SafetyReminderSendResult{}, fmt.Errorf("设置安全提醒防重复标记失败: %w", err)
		}
		if !acquired {
			return SafetyReminderSendResult{}, ErrSafetyReminderAlreadySent
		}
	}

	pngBytes, content, err := s.Preview(date, config.TopicsFile)
	if err != nil {
		if deduplicate {
			vars.RedisClient.Del(s.ctx, dedupeKey)
		}
		return SafetyReminderSendResult{}, err
	}
	if _, err := NewMessageService(s.ctx).MsgUploadImg(config.TargetChatRoomID, bytes.NewReader(pngBytes)); err != nil {
		if deduplicate {
			vars.RedisClient.Del(s.ctx, dedupeKey)
		}
		return SafetyReminderSendResult{}, fmt.Errorf("发送安全提醒图片失败: %w", err)
	}
	if deduplicate {
		vars.RedisClient.Set(s.ctx, dedupeKey, "sent", 48*time.Hour)
	}

	return SafetyReminderSendResult{
		Date:             dateValue,
		Focus:            content.Focus,
		TargetChatRoomID: config.TargetChatRoomID,
	}, nil
}
