package controller

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"wechat-robot-client/pkg/appx"
	"wechat-robot-client/pkg/safetyreminder"
	"wechat-robot-client/service"
)

type SafetyReminder struct{}

type safetyReminderSendRequest struct {
	Date string `json:"date"`
}

func NewSafetyReminderController() *SafetyReminder {
	return &SafetyReminder{}
}

func (ct *SafetyReminder) Preview(c *gin.Context) {
	config, ok := ct.authorize(c)
	if !ok {
		return
	}
	date, err := parseSafetyReminderDate(c.Query("date"))
	if err != nil {
		appx.NewResponse(c).ToInvalidResponseMsg(err.Error())
		return
	}
	pngBytes, _, err := service.NewSafetyReminderService(c).Preview(date, config.TopicsFile)
	if err != nil {
		appx.NewResponse(c).ToErrorResponse(err)
		return
	}
	c.Header("Content-Disposition", "inline; filename=safety-reminder-"+date.Format("2006-01-02")+".png")
	c.Data(http.StatusOK, "image/png", pngBytes)
}

func (ct *SafetyReminder) Send(c *gin.Context) {
	config, ok := ct.authorize(c)
	if !ok {
		return
	}
	var request safetyReminderSendRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			appx.NewResponse(c).ToInvalidResponseMsg("参数错误")
			return
		}
	}
	date, err := parseSafetyReminderDate(request.Date)
	if err != nil {
		appx.NewResponse(c).ToInvalidResponseMsg(err.Error())
		return
	}
	result, err := service.NewSafetyReminderService(c).Send(date, config, false)
	if err != nil {
		appx.NewResponse(c).ToErrorResponse(err)
		return
	}
	appx.NewResponse(c).ToResponse(result)
}

func (ct *SafetyReminder) authorize(c *gin.Context) (safetyreminder.Config, bool) {
	config, err := safetyreminder.LoadConfig()
	if err != nil {
		appx.NewResponse(c).ToErrorResponse(err)
		return config, false
	}
	provided := c.GetHeader("X-Safety-Reminder-Token")
	if config.TestToken == "" || len(provided) != len(config.TestToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(config.TestToken)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "安全提醒测试 Token 无效", "data": nil})
		return config, false
	}
	return config, true
}

func parseSafetyReminderDate(value string) (time.Time, error) {
	if value == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local), nil
	}
	date, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return time.Time{}, errors.New("日期格式错误，应为 YYYY-MM-DD")
	}
	return date, nil
}
