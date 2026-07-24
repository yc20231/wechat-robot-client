package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestRetryInitialAIStreamRetriesTransientFailureOnce(t *testing.T) {
	attempts := 0
	want := openai.ChatCompletionMessage{Content: "ok"}

	got, _, err := retryInitialAIStream(context.Background(), true, 0, func() (openai.ChatCompletionMessage, string, error) {
		attempts++
		if attempts == 1 {
			return openai.ChatCompletionMessage{}, "", fmt.Errorf("stream error: unexpected end of JSON input")
		}
		return want, "", nil
	})
	if err != nil {
		t.Fatalf("retryInitialAIStream() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if got.Content != want.Content {
		t.Fatalf("content = %q, want %q", got.Content, want.Content)
	}
}

func TestRetryInitialAIStreamDoesNotRetryWhenBlocked(t *testing.T) {
	attempts := 0
	wantErr := fmt.Errorf("stream error: %w", io.ErrUnexpectedEOF)

	_, _, err := retryInitialAIStream(context.Background(), false, 0, func() (openai.ChatCompletionMessage, string, error) {
		attempts++
		return openai.ChatCompletionMessage{}, "", wantErr
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want unexpected EOF", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetryInitialAIStreamDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	wantErr := errors.New("401 unauthorized")

	_, _, err := retryInitialAIStream(context.Background(), true, 0, func() (openai.ChatCompletionMessage, string, error) {
		attempts++
		return openai.ChatCompletionMessage{}, "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestIsRetryableAIStreamError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unexpected eof", err: io.ErrUnexpectedEOF, want: true},
		{name: "json truncated", err: errors.New("stream error: unexpected end of JSON input"), want: true},
		{name: "connection reset", err: errors.New("read: connection reset by peer"), want: true},
		{name: "unauthorized", err: errors.New("401 unauthorized"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableAIStreamError(tt.err); got != tt.want {
				t.Fatalf("isRetryableAIStreamError() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestToolBlocksAIStreamRetry(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{name: "activate skill", toolName: "activate_skill", want: false},
		{name: "read skill resource", toolName: "read_skill_resource", want: false},
		{name: "execute skill script", toolName: "execute_skill_script", want: true},
		{name: "internal action", toolName: "send_local_image", want: true},
		{name: "mcp action", toolName: "external_action", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolBlocksAIStreamRetry(tt.toolName); got != tt.want {
				t.Fatalf("toolBlocksAIStreamRetry(%q) = %t, want %t", tt.toolName, got, tt.want)
			}
		})
	}
}
