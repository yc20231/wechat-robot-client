package safetyreminder

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	templateassets "wechat-robot-client/pkg/templates/safetyreminder"
)

type Topic struct {
	Focus  string    `json:"focus"`
	Points [3]string `json:"points"`
	Slogan string    `json:"slogan"`
}

type PosterContent struct {
	Date   time.Time
	Focus  string
	Points [3]string
	Slogan string
}

func LoadTopics(path string) ([]Topic, error) {
	var (
		data []byte
		err  error
	)
	if path == "" {
		data, err = templateassets.Assets.ReadFile("topics.json")
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("读取安全提醒主题库失败: %w", err)
	}

	var topics []Topic
	if err := json.Unmarshal(data, &topics); err != nil {
		return nil, fmt.Errorf("解析安全提醒主题库失败: %w", err)
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("安全提醒主题库不能为空")
	}
	for index, topic := range topics {
		if strings.TrimSpace(topic.Focus) == "" || strings.TrimSpace(topic.Slogan) == "" {
			return nil, fmt.Errorf("安全提醒主题库第 %d 项的重点或标语为空", index+1)
		}
		for pointIndex, point := range topic.Points {
			if strings.TrimSpace(point) == "" {
				return nil, fmt.Errorf("安全提醒主题库第 %d 项的第 %d 条内容为空", index+1, pointIndex+1)
			}
		}
	}
	return topics, nil
}

func ContentForDate(date time.Time, topics []Topic) (PosterContent, error) {
	if len(topics) == 0 {
		return PosterContent{}, fmt.Errorf("安全提醒主题库不能为空")
	}
	topic := topics[(date.YearDay()-1)%len(topics)]
	return PosterContent{
		Date:   date,
		Focus:  topic.Focus,
		Points: topic.Points,
		Slogan: topic.Slogan,
	}, nil
}
