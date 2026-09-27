package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"wechat-robot-client/vars"
)

func TestProductionNoticeReuseExistingAuth(t *testing.T) {
	oldDir := vars.SkillsDir
	vars.SkillsDir = t.TempDir()
	t.Cleanup(func() { vars.SkillsDir = oldDir })
	for _, key := range []string{"PRODUCTION_NOTICE_ROBOT_TOKEN", "PRODUCTION_NOTICE_REUSE_EXISTING_AUTH", "BUSINESS_GATEWAY_URL", "BUSINESS_GATEWAY_TOKEN", "BUSINESS_GATEWAY_CONFIG_FILE"} {
		t.Setenv(key, "")
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(vars.SkillsDir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	existing := strings.Repeat("g", 32)
	write(".business-gateway.json", `{"url":"http://business-gateway:8080","token":"`+existing+`"}`)
	if productionNoticeToken() != "" {
		t.Fatal("existing config alone must not enable notices")
	}
	write(".production-notice.json", `{"reuse_existing_auth":true}`)
	if productionNoticeToken() != existing {
		t.Fatal("mounted opt-in must reuse existing gateway token")
	}
	t.Setenv("PRODUCTION_NOTICE_REUSE_EXISTING_AUTH", "false")
	if productionNoticeToken() != "" {
		t.Fatal("environment must be able to disable file opt-in")
	}
	t.Setenv("PRODUCTION_NOTICE_REUSE_EXISTING_AUTH", "true")
	t.Setenv("BUSINESS_GATEWAY_URL", "http://business-gateway:8080")
	if productionNoticeToken() != "" {
		t.Fatal("incomplete gateway environment must not fall back to file")
	}
	t.Setenv("BUSINESS_GATEWAY_TOKEN", strings.Repeat("e", 32))
	if productionNoticeToken() != strings.Repeat("e", 32) {
		t.Fatal("must respect gateway environment precedence")
	}
	write(".production-notice.json", `{"token":"`+strings.Repeat("d", 32)+`"}`)
	if productionNoticeToken() != strings.Repeat("d", 32) {
		t.Fatal("dedicated file token must take precedence")
	}
	write(".production-notice.json", `{invalid`)
	if productionNoticeToken() != "" {
		t.Fatal("malformed file must fail closed")
	}
	t.Setenv("PRODUCTION_NOTICE_ROBOT_TOKEN", strings.Repeat("t", 32))
	if productionNoticeToken() != strings.Repeat("t", 32) {
		t.Fatal("explicit environment token must take precedence")
	}
}

func TestProductionNoticeTokenAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("t", 32)
	t.Setenv("PRODUCTION_NOTICE_ROBOT_TOKEN", token)
	router := gin.New()
	router.POST("/send", ProductionNoticeSend)
	for _, tc := range []struct {
		token, body string
		status      int
	}{
		{"", "{}", 401},
		{"wrong", `{"to_wxid":"test@chatroom","content":"hello"}`, 401},
		{token, `{"to_wxid":"friend","content":"hello"}`, 400},
		{token, `{"to_wxid":"test@chatroom","content":""}`, 400},
		{token, `{"to_wxid":"test@chatroom","content":"hello"}`, 503},
	} {
		req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Production-Notice-Token", tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
}
