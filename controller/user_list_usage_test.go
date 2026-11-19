package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminExactSearchAndDeferredUserUsage(t *testing.T) {
	db := setupManageUserTestDB(t)
	user := model.User{Id: 349, Username: "case", Email: "case@example.test", Quota: 500, UsedQuota: 27, RequestCount: 9, Group: "default"}
	other := model.User{Id: 5349, Username: "case-other", Email: "second-case@example.test", Group: "default", AffCode: "other"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Type: model.LogTypeConsume, PromptTokens: 12, CompletionTokens: 3, Quota: 4, CreatedAt: 100,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		UserId: user.Id, Type: model.LogTypeConsume, PromptTokens: 5, CompletionTokens: 2, Quota: 2, CreatedAt: 200,
	}).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/search", SearchUsers)
	router.GET("/usage-summary", GetUserListUsage)
	request := func(path string) []byte {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		return recorder.Body.Bytes()
	}
	for _, tc := range []struct {
		field, keyword string
		id             int
	}{
		{"id", " 349 ", 349},
		{"username", "case", 349},
		{"email", " CASE@example.test ", 349},
		{"username", "case-other", 5349},
		{"username", "cas", 0},
		{"id", "invalid", 0},
	} {
		t.Run(tc.field+"/"+tc.keyword, func(t *testing.T) {
			var got struct {
				Success bool
				Data    struct {
					Total int
					Items []model.User
				}
			}
			body := request("/search?include_usage=false&exact_field=" + tc.field + "&keyword=" + url.QueryEscape(tc.keyword))
			require.NoError(t, common.Unmarshal(body, &got))
			require.True(t, got.Success)
			if tc.id == 0 {
				assert.Zero(t, got.Data.Total)
				assert.Empty(t, got.Data.Items)
				return
			}
			assert.Equal(t, 1, got.Data.Total)
			require.Len(t, got.Data.Items, 1)
			assert.Equal(t, tc.id, got.Data.Items[0].Id)
			assert.NotContains(t, string(body), "usage_summary")
			if tc.id == user.Id {
				assert.Equal(t, 500, got.Data.Items[0].Quota)
			}
		})
	}
	assert.Contains(t, string(request("/search?keyword=case")), `"total":2`)
	assert.Contains(t, string(request("/search?keyword=case")), `"usage_summary":`)
	assert.Contains(t, string(request("/search?exact_field=id&keyword=349&group=other")), `"total":0`)
	assert.Contains(t, string(request("/search?exact_field=group&keyword=default")), `"success":false`)

	for _, tc := range []struct {
		query string
		want  model.UserUsageSummary
	}{
		{"", model.UserUsageSummary{TokenUsed: 22, RequestCount: 9, ConsumedQuota: 27}},
		{"&usage_start=50&usage_end=150", model.UserUsageSummary{TokenUsed: 15, RequestCount: 1, ConsumedQuota: 4}},
	} {
		var got struct {
			Success bool
			Data    map[int]model.UserUsageSummary
		}
		require.NoError(t, common.Unmarshal(request("/usage-summary?ids=349,5349,9999"+tc.query), &got))
		require.True(t, got.Success)
		assert.Equal(t, map[int]model.UserUsageSummary{349: tc.want, 5349: {}}, got.Data)
	}
	assert.Contains(t, string(request("/usage-summary?ids=349&usage_start=200&usage_end=100")), `"success":false`)
	assert.Contains(t, string(request("/usage-summary?ids=349,0")), `"success":false`)
	assert.Contains(t, string(request("/usage-summary?ids="+strings.Repeat("349,", 100)+"349")), `"success":false`)
}
