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
	// Tag 非空表示天气触发的插播专题(rain/heat)，不参与常规轮换
	Tag string `json:"tag,omitempty"`
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
	topics = NormalTopics(topics)
	if len(topics) == 0 {
		return PosterContent{}, fmt.Errorf("安全提醒常规主题库不能为空")
	}
	topic := topics[cycleIndexForDate(date, len(topics))]
	return PosterContent{
		Date:   date,
		Focus:  topic.Focus,
		Points: topic.Points,
		Slogan: topic.Slogan,
	}, nil
}

// NormalTopics 过滤掉天气插播专题，仅保留常规轮换条目。
func NormalTopics(topics []Topic) []Topic {
	out := make([]Topic, 0, len(topics))
	for _, topic := range topics {
		if topic.Tag == "" {
			out = append(out, topic)
		}
	}
	return out
}

// ContentForWeather 天气优先选题: 恶劣天气插播对应专题(同类内部按天轮换)，否则走常规轮换。
func ContentForWeather(date time.Time, topics []Topic, weather *Weather) PosterContent {
	kind := weather.Kind()
	if kind != "" {
		pool := make([]Topic, 0, 2)
		for _, topic := range topics {
			if topic.Tag == kind {
				pool = append(pool, topic)
			}
		}
		if len(pool) > 0 {
			topic := pool[cycleIndexForDate(date, len(pool))]
			return PosterContent{
				Date:   date,
				Focus:  topic.Focus,
				Points: topic.Points,
				Slogan: topic.Slogan,
			}
		}
	}
	content, _ := ContentForDate(date, topics)
	return content
}

func cycleIndexForDate(date time.Time, cycleLength int) int {
	utcDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayNumber := utcDate.Unix() / int64(24*time.Hour/time.Second)
	index := int(dayNumber % int64(cycleLength))
	if index < 0 {
		index += cycleLength
	}
	return index
}
