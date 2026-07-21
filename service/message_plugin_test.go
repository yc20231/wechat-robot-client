package service

import (
	"context"
	"testing"

	pluginiface "wechat-robot-client/interface/plugin"
	"wechat-robot-client/model"
	pluginregistry "wechat-robot-client/plugin"
	"wechat-robot-client/vars"
)

type pipelineHandler struct {
	name    string
	handled bool
	runs    int
}

func (h *pipelineHandler) GetName() string                            { return h.name }
func (h *pipelineHandler) GetLabels() []string                        { return []string{"text"} }
func (h *pipelineHandler) Match(*pluginiface.MessageContext) bool     { return true }
func (h *pipelineHandler) PreAction(*pluginiface.MessageContext) bool { return true }
func (h *pipelineHandler) PostAction(*pluginiface.MessageContext)     {}
func (h *pipelineHandler) Run(ctx *pluginiface.MessageContext) {
	h.runs++
	ctx.Handled = h.handled
}

func TestProcessTextMessageStopsAfterHandledPlugin(t *testing.T) {
	previous := vars.MessagePlugin
	t.Cleanup(func() { vars.MessagePlugin = previous })
	first := &pipelineHandler{name: "business", handled: true}
	second := &pipelineHandler{name: "ai"}
	vars.MessagePlugin = pluginregistry.NewMessagePlugin()
	vars.MessagePlugin.Register(first)
	vars.MessagePlugin.Register(second)

	service := &MessageService{ctx: context.Background()}
	service.ProcessTextMessage(&model.Message{Content: "查库存"}, nil)
	if first.runs != 1 || second.runs != 0 {
		t.Fatalf("pipeline runs: first=%d second=%d", first.runs, second.runs)
	}
}
