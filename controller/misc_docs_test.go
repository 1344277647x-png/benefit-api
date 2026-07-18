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
package controller

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

func preserveDocsOption(t *testing.T) {
	t.Helper()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	original, existed := common.OptionMap["DocsContent"]
	t.Cleanup(func() {
		if existed {
			common.OptionMap["DocsContent"] = original
			return
		}
		delete(common.OptionMap, "DocsContent")
		if optionMapWasNil {
			common.OptionMap = nil
		}
	})
}

func decodeDocsResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
} {
	t.Helper()
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestDefaultDocsContentIncludesEditableGuide(t *testing.T) {
	requiredSections := []string{
		"## 快速开始",
		"## API 地址",
		"## 创建密钥",
		"## OpenAI SDK",
		"## Codex",
		"## Cherry Studio",
		"## 在线充值",
		"## 常见问题",
	}
	for _, section := range requiredSections {
		assert.Contains(t, model.DefaultDocsContent, section)
	}
	assert.Contains(t, model.DefaultDocsContent, "https://xbenefitapi.xyz/v1")
}

func TestGetDocsContentReturnsConfiguredMarkdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	preserveDocsOption(t)

	const markdown = "## Quick start\n\nUse `https://example.com/v1`."
	common.OptionMap["DocsContent"] = markdown

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/docs", nil)

	GetDocsContent(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeDocsResponse(t, recorder)
	assert.True(t, response.Success)
	assert.Empty(t, response.Message)
	assert.Equal(t, markdown, response.Data)
}

func TestUpdateOptionPublishesDocsContentImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	preserveDocsOption(t)
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
	})

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	const markdown = "# Benefit API\n\nSaved from the administrator settings."
	requestBody := []byte(`{"key":"DocsContent","value":"# Benefit API\n\nSaved from the administrator settings."}`)
	saveRecorder := httptest.NewRecorder()
	saveContext, _ := gin.CreateTestContext(saveRecorder)
	saveContext.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(requestBody))
	saveContext.Request.Header.Set("Content-Type", "application/json")

	UpdateOption(saveContext)

	require.Equal(t, http.StatusOK, saveRecorder.Code)
	var savedOption model.Option
	require.NoError(t, db.First(&savedOption, "key = ?", "DocsContent").Error)
	assert.Equal(t, markdown, savedOption.Value)

	publicRecorder := httptest.NewRecorder()
	publicContext, _ := gin.CreateTestContext(publicRecorder)
	publicContext.Request = httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	GetDocsContent(publicContext)

	require.Equal(t, http.StatusOK, publicRecorder.Code)
	response := decodeDocsResponse(t, publicRecorder)
	assert.True(t, response.Success)
	assert.Equal(t, markdown, response.Data)
}
