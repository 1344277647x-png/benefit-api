/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const emailBindTestAccessToken = "email-bind-route-test-token"

func setupEmailBindTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, *model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	model.LOG_DB = db

	user := &model.User{
		Username:    "email-bind-user",
		Password:    "unused-test-password",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
	}
	user.SetAccessToken(emailBindTestAccessToken)
	require.NoError(t, db.Create(user).Error)

	engine := gin.New()
	SetApiRouter(engine)

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return engine, db, user
}

func TestEmailBindRouteRequiresAuthentication(t *testing.T) {
	engine, _, _ := setupEmailBindTestRouter(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/oauth/email/bind",
		bytes.NewBufferString(`{"email":"visitor@example.com","code":"ABC123"}`),
	)
	request.Header.Set("Content-Type", "application/json")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestEmailBindRouteBindsEmailForAuthenticatedUser(t *testing.T) {
	engine, db, user := setupEmailBindTestRouter(t)

	const email = "Bound.User@Example.com"
	const normalizedEmail = "bound.user@example.com"
	const code = "ABC123"
	common.RegisterVerificationCodeWithKey(normalizedEmail, code, common.EmailVerificationPurpose)
	t.Cleanup(func() {
		common.DeleteKey(normalizedEmail, common.EmailVerificationPurpose)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/oauth/email/bind",
		bytes.NewBufferString(`{"email":"Bound.User@Example.com","code":"ABC123"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+emailBindTestAccessToken)

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, normalizedEmail, updated.Email)
}
