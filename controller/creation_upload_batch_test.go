package controller

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type creationUploadPart struct {
	name string
	data []byte
}

func creationBatchUploadRequest(t *testing.T, parts []creationUploadPart) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		file, err := writer.CreateFormFile("files", part.name)
		require.NoError(t, err)
		_, err = file.Write(part.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/api/creation/uploads/batch", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func creationBatchUploadRouter() *gin.Engine {
	router := gin.New()
	router.POST("/api/creation/uploads/batch", func(c *gin.Context) {
		c.Set("id", 42)
		UploadCreationAssets(c)
	})
	return router
}

func TestDeleteCreationUploadRemovesOnlyAnUnattachedOwnedAsset(t *testing.T) {
	db := setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "delete-unattached-upload")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	userDir := filepath.Join(root, "user-42")
	t.Cleanup(func() { _ = os.Remove(root) })
	t.Cleanup(func() { _ = os.Remove(userDir) })

	asset, err := service.SaveGenerationAsset(service.GenerationAssetSaveRequest{
		UserID:   42,
		Role:     "input",
		Kind:     model.GenerationKindImage,
		Reader:   bytes.NewReader(creationAccessPNG(t)),
		MaxBytes: service.GenerationAssetLimit(model.GenerationKindImage),
	})
	require.NoError(t, err)
	assetPath, err := service.GenerationAssetPath(asset)
	require.NoError(t, err)

	router := gin.New()
	router.DELETE("/api/creation/uploads/:id", func(c *gin.Context) {
		c.Set("id", 42)
		DeleteCreationUpload(c)
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/creation/uploads/"+asset.PublicID, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	_, err = model.GetGenerationAssetForUser(42, asset.PublicID)
	assert.Error(t, err)
	_, err = os.Stat(assetPath)
	assert.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&model.GenerationAsset{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUploadCreationAssetsStoresAllValidImages(t *testing.T) {
	setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "batch-upload-success")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	userDir := filepath.Join(root, "user-42")
	t.Cleanup(func() { _ = os.Remove(userDir) })

	request := creationBatchUploadRequest(t, []creationUploadPart{
		{name: "first.png", data: creationAccessPNG(t)},
		{name: "second.png", data: creationAccessPNG(t)},
	})
	response := httptest.NewRecorder()
	creationBatchUploadRouter().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Assets []model.GenerationAsset `json:"assets"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Assets, 2)
	for index := range payload.Data.Assets {
		asset := payload.Data.Assets[index]
		assert.Equal(t, "input", asset.Role)
		assert.Equal(t, "image/png", asset.MimeType)
		assert.Contains(t, asset.ContentURL, "/api/creation/assets/")
		stored, err := model.GetGenerationAssetForUser(42, asset.PublicID)
		require.NoError(t, err)
		storedAsset := *stored
		t.Cleanup(func() { _ = service.RemoveGenerationAssetFile(&storedAsset) })
	}
}

func TestUploadCreationAssetsRollsBackEarlierFilesWhenOneFileFails(t *testing.T) {
	db := setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "batch-upload-rollback")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	userDir := filepath.Join(root, "user-42")
	t.Cleanup(func() { _ = os.Remove(userDir) })

	request := creationBatchUploadRequest(t, []creationUploadPart{
		{name: "valid.png", data: creationAccessPNG(t)},
		{name: "forged.svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
	})
	response := httptest.NewRecorder()
	creationBatchUploadRouter().ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)

	var count int64
	require.NoError(t, db.Model(&model.GenerationAsset{}).Count(&count).Error)
	assert.Zero(t, count)
	entries, err := os.ReadDir(userDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFinishCreationJobWithErrorCleansAttachedInputAssets(t *testing.T) {
	db := setupCreationAssetAccessControllerTest(t)
	t.Setenv("CREATION_ENABLED", "true")
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "failed-job-input-cleanup")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	userDir := filepath.Join(root, "user-42")
	t.Cleanup(func() { _ = os.Remove(userDir) })

	job := &model.GenerationJob{
		PublicID:      "gen_failed_input_cleanup",
		UserID:        42,
		Kind:          model.GenerationKindImage,
		Protocol:      "openai-image",
		Model:         "gpt-image-2",
		Prompt:        "test",
		Status:        model.GenerationJobPending,
		ReservedBytes: service.GenerationAssetLimit(model.GenerationKindImage),
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, db.Create(job).Error)
	asset, err := service.SaveGenerationAsset(service.GenerationAssetSaveRequest{
		UserID: 42,
		JobID:  job.ID,
		Role:   "input",
		Kind:   model.GenerationKindImage,
		Reader: bytes.NewReader(creationAccessPNG(t)),
	})
	require.NoError(t, err)
	assetPath, err := service.GenerationAssetPath(asset)
	require.NoError(t, err)

	finishCreationJobWithError(job, http.StatusBadGateway, errors.New("upstream failed"))

	var stored model.GenerationAsset
	assert.Error(t, db.Where("id = ?", asset.ID).First(&stored).Error)
	_, err = os.Stat(assetPath)
	assert.Error(t, err)
	var storedJob model.GenerationJob
	require.NoError(t, db.Where("id = ?", job.ID).First(&storedJob).Error)
	assert.Equal(t, model.GenerationJobFailed, storedJob.Status)
}
