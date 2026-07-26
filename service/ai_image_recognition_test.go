package service

import (
	"encoding/json"
	"strings"
	"testing"

	"wechat-robot-client/interface/settings"
)

func TestBuildImageRecognitionParamsUsesConfiguredVisionModelAndImageURL(t *testing.T) {
	params, err := buildImageRecognitionParams(
		"这辆是什么车",
		"https://aitupian.example.com/car.jpg",
		settings.AIConfig{ImageRecognitionModel: "vision-model"},
	)
	if err != nil {
		t.Fatalf("buildImageRecognitionParams() error = %v", err)
	}
	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{
		`"model":"vision-model"`,
		`"type":"text"`,
		`这辆是什么车`,
		`"type":"image_url"`,
		`https://aitupian.example.com/car.jpg`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload %s does not contain %q", payload, want)
		}
	}
}

func TestBuildImageRecognitionParamsValidatesRequiredInput(t *testing.T) {
	tests := []struct {
		name     string
		question string
		imageURL string
		model    string
		wantErr  string
	}{
		{name: "empty question", imageURL: "https://example.com/a.jpg", model: "vision", wantErr: "图片问题不能为空"},
		{name: "empty image URL", question: "看图", model: "vision", wantErr: "图片地址不能为空"},
		{name: "empty model", question: "看图", imageURL: "https://example.com/a.jpg", wantErr: "图像识别模型不能为空"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildImageRecognitionParams(
				tt.question,
				tt.imageURL,
				settings.AIConfig{ImageRecognitionModel: tt.model},
			)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("buildImageRecognitionParams() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
