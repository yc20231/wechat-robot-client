package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wechat-robot-client/pkg/safetyreminder"
)

func main() {
	dateValue := flag.String("date", time.Now().Format("2006-01-02"), "poster date in YYYY-MM-DD format")
	outputPath := flag.String("out", "safety-reminder-preview.png", "output PNG path")
	topicsPath := flag.String("topics", "", "optional custom topics JSON path")
	flag.Parse()

	date, err := time.ParseInLocation("2006-01-02", *dateValue, time.Local)
	if err != nil {
		fatalf("日期格式错误，应为 YYYY-MM-DD: %v", err)
	}
	topics, err := safetyreminder.LoadTopics(*topicsPath)
	if err != nil {
		fatalf("%v", err)
	}
	content, err := safetyreminder.ContentForDate(date, topics)
	if err != nil {
		fatalf("%v", err)
	}
	pngBytes, err := safetyreminder.Render(context.Background(), content)
	if err != nil {
		fatalf("%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0755); err != nil {
		fatalf("创建输出目录失败: %v", err)
	}
	if err := os.WriteFile(*outputPath, pngBytes, 0644); err != nil {
		fatalf("写入预览图片失败: %v", err)
	}
	fmt.Printf("已生成 %s（%s，今日重点：%s）\n", *outputPath, date.Format("2006-01-02"), content.Focus)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
