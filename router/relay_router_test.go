package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func TestTeamCredentialCannotReachAnyRelayFamilyWhileBillingGateIsClosed(t *testing.T) {
	setupRelayRouterTestDB(t)
	user := model.User{Username: "route-team-user", Status: common.UserStatusEnabled, Group: "default", Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	key := "tmb_local_route_guard"
	require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, TeamId: 1, TeamEnabled: true, Key: key,
		Status: common.TokenStatusDisabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	SetVideoRouter(engine)
	SetDashboardRouter(engine)
	for _, route := range []struct{ method, path string }{
		{"GET", "/v1/models"}, {"GET", "/v1beta/models"}, {"GET", "/v1beta/openai/models"},
		{"POST", "/v1/chat/completions"}, {"POST", "/v1/messages"}, {"POST", "/v1/responses"},
		{"POST", "/v1/images/generations"}, {"POST", "/v1/audio/speech"}, {"POST", "/v1/embeddings"},
		{"POST", "/v1/rerank"}, {"POST", "/v1beta/models/gemini-pro:generateContent"},
		{"GET", "/v1/realtime"}, {"POST", "/suno/submit/song"}, {"POST", "/mj/submit/imagine"},
		{"POST", "/v1/videos"}, {"POST", "/kling/v1/videos/text2video"}, {"POST", "/jimeng/?Action=CVSync2AsyncSubmitTask"},
		{"GET", "/v1/dashboard/billing/usage"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			request.Header.Set("Authorization", "Bearer "+key)
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
	var stored model.User
	require.NoError(t, model.DB.First(&stored, user.Id).Error)
	assert.Equal(t, 100, stored.Quota)
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}
