package service

import (
	"errors"
	"testing"

	"wechat-robot-client/model"
)

func TestDownloadOriginalImageWithFallbackUsesOSSAfterWechatEOF(t *testing.T) {
	message := &model.Message{ID: 42, Type: model.MsgTypeImage, AttachmentUrl: "https://oss.example.com/generated.jpg"}
	want := []byte("oss-image")

	got, contentType, name, err := downloadOriginalImageWithFallback(
		message,
		func(model.Message) ([]byte, string, string, error) {
			return nil, "", "", errors.New("EOF")
		},
		func(string) ([]byte, string, string, error) {
			return want, "image/jpeg", "generated.jpg", nil
		},
	)
	if err != nil {
		t.Fatalf("downloadOriginalImageWithFallback() error = %v", err)
	}
	if string(got) != string(want) || contentType != "image/jpeg" || name != "generated.jpg" {
		t.Fatalf("got (%q, %q, %q), want (%q, image/jpeg, generated.jpg)", got, contentType, name, want)
	}
}

func TestDownloadOriginalImageWithFallbackKeepsWechatErrorWithoutOSSURL(t *testing.T) {
	wantErr := errors.New("EOF")
	_, _, _, err := downloadOriginalImageWithFallback(
		&model.Message{ID: 42, Type: model.MsgTypeImage},
		func(model.Message) ([]byte, string, string, error) { return nil, "", "", wantErr },
		func(string) ([]byte, string, string, error) {
			t.Fatal("OSS fallback should not be called")
			return nil, "", "", nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped EOF", err)
	}
}
