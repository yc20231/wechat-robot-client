package startup

import (
	"testing"

	"wechat-robot-client/vars"
)

func TestBusinessRouterRegisteredBeforeAI(t *testing.T) {
	previous := vars.MessagePlugin
	t.Cleanup(func() { vars.MessagePlugin = previous })
	t.Setenv("BUSINESS_GATEWAY_URL", "")
	RegisterMessagePlugin()
	if len(vars.MessagePlugin.Plugins) < 2 {
		t.Fatalf("registered plugins = %d", len(vars.MessagePlugin.Plugins))
	}
	if got := vars.MessagePlugin.Plugins[0].GetName(); got != "BusinessRouter" {
		t.Fatalf("first plugin = %q", got)
	}
	if got := vars.MessagePlugin.Plugins[1].GetName(); got != "ChatRoomAIChat" {
		t.Fatalf("second plugin = %q", got)
	}
}
