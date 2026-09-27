package controller

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"wechat-robot-client/service"
	"wechat-robot-client/vars"
)

// Keep upgrades disabled by default. Existing gateway auth may be reused only
// with an explicit opt-in; a dedicated token still takes precedence.
func productionNoticeToken() string {
	token := strings.TrimSpace(os.Getenv("PRODUCTION_NOTICE_ROBOT_TOKEN"))
	if token != "" {
		return token
	}
	dir := vars.SkillsDir
	if dir == "" {
		dir = "/data/skills"
	}
	var cfg struct {
		Token             string `json:"token"`
		ReuseExistingAuth bool   `json:"reuse_existing_auth"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".production-notice.json"))
	if err == nil {
		if json.Unmarshal(raw, &cfg) != nil {
			return ""
		}
	} else if !os.IsNotExist(err) {
		return ""
	}
	if token = strings.TrimSpace(cfg.Token); token != "" {
		return token
	}
	if value := strings.TrimSpace(os.Getenv("PRODUCTION_NOTICE_REUSE_EXISTING_AUTH")); value != "" {
		cfg.ReuseExistingAuth, err = strconv.ParseBool(value)
		if err != nil {
			return ""
		}
	}
	if !cfg.ReuseExistingAuth {
		return ""
	}
	// Match the existing business router's environment/file precedence.
	if strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_URL")) != "" {
		return strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_TOKEN"))
	}
	path := strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_CONFIG_FILE"))
	if path == "" {
		path = filepath.Join(dir, ".business-gateway.json")
	}
	var gateway struct {
		Token string `json:"token"`
	}
	raw, err = os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &gateway) != nil {
		return ""
	}
	return strings.TrimSpace(gateway.Token)
}

func productionNoticeAuthorized(c *gin.Context) bool {
	token := productionNoticeToken()
	provided := c.GetHeader("X-Production-Notice-Token")
	if len(token) < 32 || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "生产通知未启用或凭据无效"})
		return false
	}
	return true
}
func ProductionNoticeHealth(c *gin.Context) {
	if !productionNoticeAuthorized(c) {
		return
	}
	ready := vars.RobotRuntime != nil && vars.RobotRuntime.Client != nil && vars.RobotRuntime.IsLoggedIn()
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ready": ready}})
}
func ProductionNoticeSend(c *gin.Context) {
	if !productionNoticeAuthorized(c) {
		return
	}
	var in struct {
		ToWxid  string `json:"to_wxid"`
		Content string `json:"content"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	if c.ShouldBindJSON(&in) != nil || !strings.HasSuffix(in.ToWxid, "@chatroom") || len(in.ToWxid) > 200 || strings.TrimSpace(in.Content) == "" || len(in.Content) > 6000 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "生产通知参数无效"})
		return
	}
	if vars.RobotRuntime == nil || vars.RobotRuntime.Client == nil || !vars.RobotRuntime.IsLoggedIn() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "机器人未登录"})
		return
	}
	if err := service.NewMessageService(c).SendProductionNoticeMessage(in.ToWxid, in.Content); err != nil {
		// Do not expose protocol details or credentials. Caller must mark unknown.
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "未获得有效发送回执，请核对微信群"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"accepted": true}})
}
