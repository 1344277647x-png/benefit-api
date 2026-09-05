package controller

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveOpenAICreationImagesStoresPartialResultsAndReleasesReservation(t *testing.T) {
	db := setupCreationAssetAccessControllerTest(t)
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "creation-partial-results")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	userDir := filepath.Join(root, "user-42")
	t.Cleanup(func() { _ = os.Remove(userDir) })

	parameters, err := common.Marshal(map[string]any{"count": 4})
	require.NoError(t, err)
	job := &model.GenerationJob{
		PublicID:      "gen_partial_results",
		UserID:        42,
		Kind:          model.GenerationKindImage,
		Protocol:      "openai-image",
		Model:         "gpt-image-2",
		Prompt:        "test",
		Parameters:    string(parameters),
		Status:        model.GenerationJobPending,
		ReservedBytes: service.GenerationAssetLimit(model.GenerationKindImage) * 4,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, db.Create(job).Error)
	encoded := base64.StdEncoding.EncodeToString(creationAccessPNG(t))
	body, err := common.Marshal(map[string]any{
		"data": []map[string]string{
			{"b64_json": encoded},
			{"b64_json": encoded},
		},
	})
	require.NoError(t, err)

	assets, err := archiveOpenAICreationImages(job, body)
	require.NoError(t, err)
	require.Len(t, assets, 2)
	for index := range assets {
		asset := assets[index]
		t.Cleanup(func() { _ = service.RemoveGenerationAssetFile(&asset) })
	}
	require.NoError(t, model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", ""))

	stored, err := model.GetGenerationJobForUser(42, job.PublicID)
	require.NoError(t, err)
	decorateCreationJob(stored)
	assert.Equal(t, 4, stored.RequestedCount)
	assert.Equal(t, 2, stored.ResultCount)
	assert.Zero(t, stored.ReservedBytes)
	usage, err := model.GetGenerationStorageUsage(42)
	require.NoError(t, err)
	assert.Equal(t, assets[0].SizeBytes+assets[1].SizeBytes, usage.UserBytes)
}

func TestArchiveOpenAICreationImagesReturnsNoAssetsForEmptyResponse(t *testing.T) {
	body, err := common.Marshal(map[string]any{"data": []any{}})
	require.NoError(t, err)

	assets, err := archiveOpenAICreationImages(&model.GenerationJob{UserID: 42}, body)
	require.NoError(t, err)
	assert.Empty(t, assets)
}
