package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wechat-robot-client/interface/settings"
)

func TestDownloadImageAsDataURLInlinesImageBytes(t *testing.T) {
	imageBytes := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	got, err := downloadImageAsDataURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("downloadImageAsDataURL() error = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
		t.Fatalf("data URL prefix = %q", got)
	}

	encoded := strings.TrimPrefix(got, "data:image/jpeg;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}
	if !bytes.Equal(decoded, imageBytes) {
		t.Fatalf("decoded bytes = %v, want %v", decoded, imageBytes)
	}
}

func TestBuildImageDataURLUsesDownloadedImageBytes(t *testing.T) {
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	got, err := buildImageDataURL(imageBytes, "image/png; charset=binary")
	if err != nil {
		t.Fatalf("buildImageDataURL() error = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("data URL prefix = %q", got)
	}
	encoded := strings.TrimPrefix(got, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode error = %v", err)
	}
	if !bytes.Equal(decoded, imageBytes) {
		t.Fatalf("decoded bytes = %v, want %v", decoded, imageBytes)
	}
}

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
