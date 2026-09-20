package plugins

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	images  int
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

func (s *recordingMessageService) MsgUploadImg(toWxID string, image io.Reader) (*model.Message, error) {
	s.to = toWxID
	s.images++
	if image != nil {
		_, _ = io.Copy(io.Discard, image)
	}
	return &model.Message{}, nil
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
			MsgId:         123,
			IsChatRoom:    true,
			IsAtMe:        true,
			FromWxID:      "group@chatroom",
			SenderWxID:    "member-wxid",
			MessageSource: `<msgsource><atuserlist>robot-wxid,target-wxid,target-wxid</atuserlist></msgsource>`,
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
	if got := routeClient.request.MentionedWxIDs; len(got) != 2 || got[0] != "robot-wxid" || got[1] != "target-wxid" {
		t.Fatalf("mentioned wxids = %#v", got)
	}
}

func TestBusinessRouterPreservesInventoryLeadingNewline(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: true, Reply: "\n    056 库存\n#2130晶包    2件"}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if messages.content != routeClient.response.Reply {
		t.Fatalf("reply whitespace changed: got %q, want %q", messages.content, routeClient.response.Reply)
	}
}

func TestBusinessRouterMentionsGatewayReplyTargets(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{Handled: true, Reply: "权限已更新", ReplyAtWxIDs: []string{"target-wxid", "member-wxid", "target-wxid"}}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)

	(&BusinessRouterPlugin{client: routeClient}).Run(ctx)
	if len(messages.at) != 2 || messages.at[0] != "member-wxid" || messages.at[1] != "target-wxid" {
		t.Fatalf("reply mentions = %#v", messages.at)
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

func TestBusinessRouterSendsCostImageWithoutText(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{
		Handled: true,
		Reply:   "【40厚白】配料成本\n共计：2012.5斤 = 8434.5元",
		Image: &BusinessRouteImage{
			Title: "40厚白",
			Cells: [][]string{{"阳强配料生产安排单", "", "", ""}},
			Cost: &BusinessRouteCost{
				Status:            "complete",
				TotalWeightJin:    "2012.5",
				KnownCost:         "8434.5",
				AverageCostPerJin: strPtr("4.191"),
				Regions: []BusinessRouteCostRegion{{
					Name:           "A",
					TotalWeightJin: "2012.5",
					Rows: []BusinessRouteCostRow{{
						MaterialName: "7000F", RawQuantity: "15", WeightJin: "300",
						Cost: strPtr("1290"), UnitPrice: strPtr("4.3"),
					}},
				}},
			},
		},
	}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)
	var captured string
	var width int
	plugin := &BusinessRouterPlugin{
		client: routeClient,
		captureHTML: func(_ context.Context, html string, captureWidth int) ([]byte, error) {
			captured = html
			width = captureWidth
			return []byte("png"), nil
		},
	}

	plugin.Run(ctx)
	if !ctx.Handled || messages.images != 1 {
		t.Fatalf("image not sent: handled=%t images=%d", ctx.Handled, messages.images)
	}
	if messages.content != "" {
		t.Fatalf("text still sent with cost image: %q", messages.content)
	}
	if width < 1400 {
		t.Fatalf("capture width = %d, want editor viewport", width)
	}
	for _, want := range []string{"配料成本", "实时预览", "8382元/吨", "7000F", "撤销"} {
		if !strings.Contains(captured, want) {
			t.Fatalf("captured html missing %q", want)
		}
	}
}

func TestBusinessRouterFallsBackToTextWhenImageFails(t *testing.T) {
	routeClient := &fakeRouteClient{response: BusinessRouteResponse{
		Handled: true,
		Reply:   "【40厚白】配料成本",
		Image:   &BusinessRouteImage{Title: "40厚白", Cells: [][]string{{"表"}}},
	}}
	messages := &recordingMessageService{}
	ctx := businessContext(messages)
	plugin := &BusinessRouterPlugin{
		client: routeClient,
		captureHTML: func(context.Context, string, int) ([]byte, error) {
			return nil, errors.New("chrome missing")
		},
	}

	plugin.Run(ctx)
	if messages.images != 0 || messages.content != "【40厚白】配料成本" {
		t.Fatalf("fallback failed: images=%d content=%q", messages.images, messages.content)
	}
}

func strPtr(value string) *string { return &value }

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
