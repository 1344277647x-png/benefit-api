package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenericOptionRejectsDedicatedAuditAndLotterySettings(t *testing.T) {
	for _, key := range []string{"content_audit.enabled", "content_audit.privacy_ready", "lottery_setting.rule_version", "lottery_setting.regular_weights"} {
		t.Run(key, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/option", strings.NewReader(`{"key":"`+key+`","value":true}`))
			UpdateOption(c)
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			assert.Contains(t, recorder.Body.String(), "专用设置接口")
		})
	}
}
