/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

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
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCreationAssetAccessControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.SessionSecret = "creation-asset-controller-test-secret"
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.GenerationJob{}, &model.GenerationAsset{}))

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func creationAccessPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	picture.Set(0, 0, color.NRGBA{R: 24, G: 173, B: 137, A: 255})
	require.NoError(t, png.Encode(&output, picture))
	return output.Bytes()
}

func TestCreationAssetTicketPreservesOwnerAndHTTPRange(t *testing.T) {
	setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	assetRoot := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", strings.ReplaceAll(t.Name(), "/", "_"))
	require.NoError(t, os.MkdirAll(assetRoot, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", assetRoot)
	var asset *model.GenerationAsset
	t.Cleanup(func() {
		// The saved file is removed explicitly; the now-empty user directory and
		// test directory are removed individually to avoid recursive cleanup.
		if asset != nil {
			_ = service.RemoveGenerationAssetFile(asset)
		}
		if asset != nil {
			_ = os.Remove(filepath.Dir(filepath.Join(assetRoot, asset.RelativePath)))
		}
		_ = os.Remove(assetRoot)
	})

	var err error
	asset, err = service.SaveGenerationAsset(service.GenerationAssetSaveRequest{
		UserID: 42,
		Role:   "output",
		Kind:   model.GenerationKindImage,
		Reader: bytes.NewReader(creationAccessPNG(t)),
	})
	require.NoError(t, err)

	ticket, _, err := service.IssueCreationAssetAccessToken(42, asset.PublicID, false)
	require.NoError(t, err)
	router := gin.New()
	router.GET("/api/creation/assets/:id/content", GetCreationAssetContent)

	request := httptest.NewRequest(http.MethodGet, "/api/creation/assets/"+asset.PublicID+"/content?ticket="+ticket, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "image/png", response.Header().Get("Content-Type"))
	require.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, creationAccessPNG(t), response.Body.Bytes())

	rangeRequest := httptest.NewRequest(http.MethodGet, "/api/creation/assets/"+asset.PublicID+"/content?ticket="+ticket, nil)
	rangeRequest.Header.Set("Range", "bytes=0-3")
	rangeResponse := httptest.NewRecorder()
	router.ServeHTTP(rangeResponse, rangeRequest)
	require.Equal(t, http.StatusPartialContent, rangeResponse.Code)
	require.True(t, strings.HasPrefix(rangeResponse.Header().Get("Content-Range"), "bytes 0-3/"))
	require.Len(t, rangeResponse.Body.Bytes(), 4)

	wrongUserTicket, _, err := service.IssueCreationAssetAccessToken(7, asset.PublicID, false)
	require.NoError(t, err)
	wrongRequest := httptest.NewRequest(http.MethodGet, "/api/creation/assets/"+asset.PublicID+"/content?ticket="+wrongUserTicket, nil)
	wrongResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongResponse, wrongRequest)
	require.Equal(t, http.StatusNotFound, wrongResponse.Code)
}

func TestCreationAssetContentRequiresAuthenticationOrTicket(t *testing.T) {
	setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	router := gin.New()
	router.GET("/api/creation/assets/:id/content", GetCreationAssetContent)

	request := httptest.NewRequest(http.MethodGet, "/api/creation/assets/asset_missing/content", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}
