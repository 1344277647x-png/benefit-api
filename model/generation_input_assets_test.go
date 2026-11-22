package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepareGenerationInputAssetTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&GenerationJob{}, &GenerationAsset{}))
	require.NoError(t, DB.Exec("DELETE FROM generation_assets").Error)
	require.NoError(t, DB.Exec("DELETE FROM generation_jobs").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM generation_assets").Error
		_ = DB.Exec("DELETE FROM generation_jobs").Error
	})
}

func createGenerationJobFixture(t *testing.T, suffix string, userID int) *GenerationJob {
	t.Helper()
	job := &GenerationJob{
		PublicID:  "gen_" + suffix,
		UserID:    userID,
		Kind:      GenerationKindImage,
		Protocol:  "openai-image",
		Model:     "gpt-image-2",
		Prompt:    "test",
		Status:    GenerationJobPending,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, DB.Create(job).Error)
	return job
}

func createGenerationInputFixture(t *testing.T, suffix string, userID int, jobID int64, expiresAt int64) *GenerationAsset {
	t.Helper()
	asset := &GenerationAsset{
		PublicID:     "asset_" + suffix,
		JobID:        jobID,
		UserID:       userID,
		Role:         "input",
		RelativePath: fmt.Sprintf("user-%d/%s.png", userID, suffix),
		MimeType:     "image/png",
		SizeBytes:    128,
		Status:       "ready",
		ExpiresAt:    expiresAt,
	}
	require.NoError(t, DB.Create(asset).Error)
	return asset
}

func TestAttachGenerationInputAssetsBindsAllAssetsInClientOrder(t *testing.T) {
	prepareGenerationInputAssetTest(t)
	job := createGenerationJobFixture(t, "target", 42)
	now := time.Now().Add(time.Hour).Unix()
	first := createGenerationInputFixture(t, "first", 42, 0, now)
	second := createGenerationInputFixture(t, "second", 42, 0, now)

	attached, err := AttachGenerationInputAssets(42, []string{second.PublicID, first.PublicID}, job.ID)
	require.NoError(t, err)
	require.Len(t, attached, 2)
	assert.Equal(t, []string{second.PublicID, first.PublicID}, []string{attached[0].PublicID, attached[1].PublicID})
	assert.Equal(t, job.ID, attached[0].JobID)
	assert.Equal(t, job.ID, attached[1].JobID)
}

func TestAttachGenerationInputAssetsDeduplicatesIDsWithoutChangingOrder(t *testing.T) {
	prepareGenerationInputAssetTest(t)
	job := createGenerationJobFixture(t, "target", 42)
	now := time.Now().Add(time.Hour).Unix()
	first := createGenerationInputFixture(t, "first", 42, 0, now)
	second := createGenerationInputFixture(t, "second", 42, 0, now)

	attached, err := AttachGenerationInputAssets(42, []string{second.PublicID, second.PublicID, first.PublicID}, job.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{second.PublicID, first.PublicID}, []string{attached[0].PublicID, attached[1].PublicID})
	assert.Equal(t, job.ID, attached[0].JobID)
	assert.Equal(t, job.ID, attached[1].JobID)
}

func TestAttachGenerationInputAssetsRollsBackWholeBatch(t *testing.T) {
	prepareGenerationInputAssetTest(t)
	job := createGenerationJobFixture(t, "target", 42)
	now := time.Now().Add(time.Hour).Unix()
	first := createGenerationInputFixture(t, "first", 42, 0, now)
	foreign := createGenerationInputFixture(t, "foreign", 7, 0, now)

	_, err := AttachGenerationInputAssets(42, []string{first.PublicID, foreign.PublicID}, job.ID)
	require.Error(t, err)

	var stored GenerationAsset
	require.NoError(t, DB.Where("id = ?", first.ID).First(&stored).Error)
	assert.Zero(t, stored.JobID)
}

func TestAttachGenerationInputAssetsRejectsExpiredAndPreviouslyBoundAssets(t *testing.T) {
	prepareGenerationInputAssetTest(t)
	target := createGenerationJobFixture(t, "target", 42)
	other := createGenerationJobFixture(t, "other", 42)
	expired := createGenerationInputFixture(t, "expired", 42, 0, time.Now().Add(-time.Minute).Unix())
	bound := createGenerationInputFixture(t, "bound", 42, other.ID, time.Now().Add(time.Hour).Unix())

	_, err := AttachGenerationInputAssets(42, []string{expired.PublicID}, target.ID)
	require.ErrorContains(t, err, "expired")
	_, err = AttachGenerationInputAssets(42, []string{bound.PublicID}, target.ID)
	require.ErrorContains(t, err, "another job")
}
