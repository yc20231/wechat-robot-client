package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	pluginiface "wechat-robot-client/interface/plugin"
	"wechat-robot-client/model"
)

type fakeRouteClient struct {
	response BusinessRouteResponse
	err      error
	request  BusinessRouteRequest
}

func (f *fakeRouteClient) Route(_ context.Context, req BusinessRouteRequest) (BusinessRouteResponse, error) {
	f.request = req
	return f.response, f.err
}

type recordingMessageService struct {
	pluginiface.MessageServiceIface
	to      string
	content string
	at      []string
}

type blacklistedMessageService struct {
	recordingMessageService
}

func (s *recordingMessageService) SendTextMessage(toWxID, content string, at ...string) error {
	s.to = toWxID
	s.content = content
	s.at = at
	return nil
}

func (s *recordingMessageService) GetChatRoomMember(_, _ string) (*model.ChatRoomMember, error) {
	return &model.ChatRoomMember{}, nil
}

func (s *blacklistedMessageService) GetChatRoomMember(_, _ string) (*model.ChatRoomMember, error) {
	blacklisted := true
	return &model.ChatRoomMember{IsBlacklisted: &blacklisted}, nil
}

func businessContext(service pluginiface.MessageServiceIface) *pluginiface.MessageContext {
	return &pluginiface.MessageContext{
		Context: context.Background(),
		Message: &model.Message{
			MsgId:      123,
			IsChatRoom: true,
			IsAtMe:     true,
			FromWxID:   "group@chatroom",
			SenderWxID: "member-wxid",
		},
		MessageContent: "查库存",
		MessageService: service,
	}
}

func TestBusinessRouterStopsHandledMessage(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: true, Reply: "库存结果"}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)
	plugin := &BusinessRouterPlugin{client: routeClient}

	plugin.Run(ctx)
	if !ctx.Handled || messages.content != "库存结果" || messages.to != "group@chatroom" {
		t.Fatalf("message was not handled: ctx=%+v service=%+v", ctx, messages)
	}
	if len(messages.at) != 1 || messages.at[0] != "member-wxid" {
		t.Fatalf("reply mention = %#v", messages.at)
	}
}

func TestBusinessRouterAllowsNonBusinessMessageToContinue(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: false}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if ctx.Handled || messages.content != "" {
		t.Fatalf("non-business message was stopped: ctx=%+v content=%q", ctx, messages.content)
	}
}

func TestBusinessRouterSkipsMessageWithoutMention(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: true, Reply: "不应发送"}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)
	ctx.Message.IsAtMe = false

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if ctx.Handled || messages.content != "" {
		t.Fatalf("message without mention was handled: ctx=%+v content=%q", ctx, messages.content)
	}
}

func TestBusinessRouterRespectsMemberBlacklist(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: true, Reply: "不应发送"}}
	messages := &blacklistedMessageService{}
	ctx := businessContext(messages)

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if ctx.Handled || messages.content != "" || routeClient.request.MessageID != 0 {
		t.Fatalf("blacklisted member reached business router: ctx=%+v request=%+v", ctx, routeClient.request)
	}
}

func TestBusinessRouterFailsClosed(t *testing.T) {
	routeClient := &fakeRouteClient{err: errors.New("gateway unavailable")}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if !ctx.Handled || messages.content != defaultBusinessRouteError {
		t.Fatalf("gateway failure did not fail closed: handled=%t content=%q", ctx.Handled, messages.content)
	}
}

func TestLoadBusinessRouterConfigFromMountedFile(t *testing.T) {
	t.Setenv("BUSINESS_GATEWAY_URL", "")
	path := filepath.Join(t.TempDir(), ".business-gateway.json")
	t.Setenv("BUSINESS_GATEWAY_CONFIG_FILE", path)
	if err := os.WriteFile(path, []byte(`{"url":"http://business-gateway:8080","token":"secret","timeout_sec":7}`), 0o600); err != nil {
		t.Fatal(err)
	}

	config, configured, err := loadBusinessRouterConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !configured || config.URL != "http://business-gateway:8080" || config.Token != "secret" || config.TimeoutSec != 7 {
		t.Fatalf("unexpected config: configured=%t config=%+v", configured, config)
	}
}

func TestInvalidConfiguredBusinessRouterFailsClosed(t *testing.T) {
	t.Setenv("BUSINESS_GATEWAY_URL", "http://business-gateway:8080")
	t.Setenv("BUSINESS_GATEWAY_TOKEN", "")
	plugin := NewBusinessRouterPlugin().(*BusinessRouterPlugin)
	if plugin.configErr == nil || plugin.client != nil {
		t.Fatalf("invalid configured plugin = %+v", plugin)
	}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)
	plugin.Run(ctx)
	if !ctx.Handled || messages.content != defaultBusinessRouteError {
		t.Fatalf("invalid config did not fail closed: handled=%t content=%q", ctx.Handled, messages.content)
	}
}
