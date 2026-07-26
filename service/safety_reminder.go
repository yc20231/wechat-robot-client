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
	Date    string                       `json:"date"`
	Focus   string                       `json:"focus"`
	Targets []SafetyReminderTargetResult `json:"targets"`
}

type SafetyReminderTargetResult struct {
	TargetChatRoomID string `json:"target_chat_room_id"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
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

// Send renders one poster and uploads it to every configured group.
// deduplicate must be true for scheduled sends.
func (s *SafetyReminderService) Send(date time.Time, config safetyreminder.Config, deduplicate bool) (SafetyReminderSendResult, error) {
	if err := config.ValidateForSend(); err != nil {
		return SafetyReminderSendResult{}, err
	}

	dateValue := date.Format("2006-01-02")
	targets := config.Targets()
	result := SafetyReminderSendResult{
		Date:    dateValue,
		Targets: make([]SafetyReminderTargetResult, len(targets)),
	}
	pending := make([]int, 0, len(targets))
	var sendErrors []error

	if deduplicate && vars.RedisClient == nil {
		return result, errors.New("Redis 未初始化，无法保证安全提醒不重复发送")
	}
	for index, target := range targets {
		result.Targets[index].TargetChatRoomID = target
		if !deduplicate {
			pending = append(pending, index)
			continue
		}

		dedupeKey := safetyReminderDedupeKey(dateValue, target)
		acquired, err := vars.RedisClient.SetNX(s.ctx, dedupeKey, "sending", 48*time.Hour).Result()
		if err != nil {
			wrappedErr := fmt.Errorf("群 %s 设置安全提醒防重复标记失败: %w", target, err)
			result.Targets[index].Status = "failed"
			result.Targets[index].Error = wrappedErr.Error()
			sendErrors = append(sendErrors, wrappedErr)
			continue
		}
		if !acquired {
			result.Targets[index].Status = "already_sent"
			continue
		}
		pending = append(pending, index)
	}

	if len(pending) == 0 {
		if len(sendErrors) > 0 {
			return result, errors.Join(sendErrors...)
		}
		return result, ErrSafetyReminderAlreadySent
	}

	pngBytes, content, err := s.Preview(date, config.TopicsFile)
	if err != nil {
		for _, index := range pending {
			target := targets[index]
			if deduplicate {
				vars.RedisClient.Del(s.ctx, safetyReminderDedupeKey(dateValue, target))
			}
			result.Targets[index].Status = "failed"
			result.Targets[index].Error = err.Error()
		}
		return result, err
	}
	result.Focus = content.Focus

	messageService := NewMessageService(s.ctx)
	for _, index := range pending {
		target := targets[index]
		dedupeKey := safetyReminderDedupeKey(dateValue, target)
		if _, err := messageService.MsgUploadImg(target, bytes.NewReader(pngBytes)); err != nil {
			if deduplicate {
				vars.RedisClient.Del(s.ctx, dedupeKey)
			}
			wrappedErr := fmt.Errorf("群 %s 发送安全提醒图片失败: %w", target, err)
			result.Targets[index].Status = "failed"
			result.Targets[index].Error = wrappedErr.Error()
			sendErrors = append(sendErrors, wrappedErr)
			continue
		}
		if deduplicate {
			vars.RedisClient.Set(s.ctx, dedupeKey, "sent", 48*time.Hour)
		}
		result.Targets[index].Status = "sent"
	}

	if len(sendErrors) > 0 {
		return result, errors.Join(sendErrors...)
	}
	return result, nil
}

func safetyReminderDedupeKey(dateValue, targetChatRoomID string) string {
	return fmt.Sprintf("safety-reminder:sent:%s:%s", dateValue, targetChatRoomID)
}
