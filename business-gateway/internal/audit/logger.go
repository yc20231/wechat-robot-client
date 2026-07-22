package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time         time.Time `json:"time"`
	Action       string    `json:"action"`
	ActorWxID    string    `json:"actor_wxid"`
	TargetWxID   string    `json:"target_wxid,omitempty"`
	GroupID      string    `json:"group_id"`
	MessageID    int64     `json:"message_id,omitempty"`
	PreviousType string    `json:"previous_type,omitempty"`
	NewType      string    `json:"new_type,omitempty"`
	CustomerCode string    `json:"customer_code,omitempty"`
}

type Logger interface {
	Record(event Event) error
}

type FileLogger struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
}

func NewFileLogger(path string) *FileLogger {
	return &FileLogger{path: path, now: time.Now}
}

func (l *FileLogger) Record(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event.Time.IsZero() {
		event.Time = l.now().UTC()
	}
	content, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("编码审计事件: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("创建审计目录: %w", err)
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("打开审计文件: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("设置审计文件权限: %w", err)
	}
	if _, err := file.Write(append(content, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("写入审计事件: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("同步审计事件: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭审计文件: %w", err)
	}
	return nil
}
