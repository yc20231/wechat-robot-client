package common_cron

import (
	"errors"
	"log"
	"time"

	"wechat-robot-client/pkg/safetyreminder"
	"wechat-robot-client/service"
	"wechat-robot-client/vars"
)

type SafetyReminderCron struct {
	CronManager *CronManager
	config      safetyreminder.Config
}

func NewSafetyReminderCron(cronManager *CronManager) vars.CommonCronInstance {
	config, err := safetyreminder.LoadConfig()
	if err != nil {
		log.Printf("加载安全提醒配置失败: %v", err)
	}
	return &SafetyReminderCron{CronManager: cronManager, config: config}
}

func (cron *SafetyReminderCron) IsActive() bool {
	return cron.config.Enabled
}

func (cron *SafetyReminderCron) Cron() error {
	now := time.Now()
	if !cron.config.SendOnWeekends && (now.Weekday() == time.Saturday || now.Weekday() == time.Sunday) {
		log.Println("安全提醒已跳过周末")
		return nil
	}
	result, err := service.NewSafetyReminderService(cron.CronManager.ctx).Send(now, cron.config, true)
	if errors.Is(err, service.ErrSafetyReminderAlreadySent) {
		log.Printf("%s，跳过重复任务", err)
		return nil
	}
	for _, target := range result.Targets {
		switch target.Status {
		case "sent":
			log.Printf("安全提醒发送成功: 日期=%s 重点=%s 群=%s", result.Date, result.Focus, target.TargetChatRoomID)
		case "already_sent":
			log.Printf("今日安全提醒已发送，跳过群: %s", target.TargetChatRoomID)
		case "failed":
			log.Printf("安全提醒发送失败: 群=%s 错误=%s", target.TargetChatRoomID, target.Error)
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func (cron *SafetyReminderCron) Register() {
	if !cron.IsActive() {
		log.Println("每日安全提醒任务未启用")
		return
	}
	if err := cron.config.ValidateForSend(); err != nil {
		log.Printf("每日安全提醒配置无效: %v", err)
		return
	}
	if err := cron.CronManager.AddJob(vars.SafetyReminderCron, cron.config.Cron, func() {
		log.Println("开始每日安全提醒任务")
		if err := cron.Cron(); err != nil {
			log.Printf("每日安全提醒任务执行失败: %v", err)
		}
	}); err != nil {
		log.Printf("每日安全提醒任务注册失败: %v", err)
		return
	}
	log.Printf("每日安全提醒任务初始化成功，执行时间: %s", cron.config.Cron)
}
