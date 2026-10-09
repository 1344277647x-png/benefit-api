package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagementResponsesExcludeNotificationCredentials(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Username: "notification-owner", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default",
		Setting: `{"webhook_secret":"test-webhook-secret","gotify_token":"test-gotify-token","bark_url":"https://bark.example/private-key"}`}
	require.NoError(t, db.Create(&user).Error)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{"list", GetAllUsers}, {"search", SearchUsers}, {"detail", GetUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(r)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/user?keyword=notification-owner", nil)
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
			c.Set("role", common.RoleRootUser)
			tc.handler(c)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
			require.True(t, response.Success, r.Body.String())
			assert.NotContains(t, r.Body.String(), "test-webhook-secret")
			assert.NotContains(t, r.Body.String(), "test-gotify-token")
			assert.NotContains(t, r.Body.String(), "private-key")
		})
	}
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, user.Setting, stored.Setting, "owner settings must remain intact")
}
