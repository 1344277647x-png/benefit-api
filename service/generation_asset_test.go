package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generationPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.NRGBA{R: 24, G: 173, B: 137, A: 255})
	require.NoError(t, png.Encode(&output, picture))
	return output.Bytes()
}

func configureGenerationAssetTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GENERATION_ASSET_ROOT", root)
	require.NoError(t, model.DB.Exec("DELETE FROM generation_assets").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM generation_jobs").Error)
	return root
}

func TestSaveGenerationAssetValidatesAndStoresImage(t *testing.T) {
	root := configureGenerationAssetTest(t)
	asset, err := SaveGenerationAsset(GenerationAssetSaveRequest{
		UserID: 42,
		Role:   "input",
		Kind:   model.GenerationKindImage,
		Reader: bytes.NewReader(generationPNG(t)),
	})
	require.NoError(t, err)
	assert.Equal(t, "image/png", asset.MimeType)
	assert.Equal(t, "input", asset.Role)
	assert.Positive(t, asset.SizeBytes)

	path, err := GenerationAssetPath(asset)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.FileExists(t, path)
	assert.Contains(t, path, filepath.Join(root, "user-42"))

	usage, err := model.GetGenerationStorageUsage(42)
	require.NoError(t, err)
	assert.Equal(t, asset.SizeBytes, usage.UserBytes)
}

func TestSaveGenerationAssetRejectsForgedAndOversizedContent(t *testing.T) {
	configureGenerationAssetTest(t)

	_, err := SaveGenerationAsset(GenerationAssetSaveRequest{
		UserID: 7,
		Role:   "input",
		Kind:   model.GenerationKindImage,
		Reader: bytes.NewBufferString(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
	})
	assert.ErrorIs(t, err, ErrUnsupportedAssetType)

	_, err = SaveGenerationAsset(GenerationAssetSaveRequest{
		UserID:   7,
		Role:     "input",
		Kind:     model.GenerationKindImage,
		Reader:   bytes.NewReader(generationPNG(t)),
		MaxBytes: 8,
	})
	assert.ErrorIs(t, err, ErrGenerationAssetTooLarge)
}

func TestGenerationAssetPathRejectsTraversal(t *testing.T) {
	configureGenerationAssetTest(t)
	_, err := GenerationAssetPath(&model.GenerationAsset{RelativePath: "../outside.png"})
	assert.ErrorContains(t, err, "escapes")

	absolute := filepath.Join(string(filepath.Separator), "outside.png")
	_, err = GenerationAssetPath(&model.GenerationAsset{RelativePath: absolute})
	assert.ErrorContains(t, err, "absolute")
}

func TestSaveRemoteGenerationAssetRequiresHTTPS(t *testing.T) {
	configureGenerationAssetTest(t)
	_, err := SaveRemoteGenerationAsset(context.Background(), "http://127.0.0.1/image.png", GenerationAssetSaveRequest{
		UserID: 1,
		Role:   "output",
		Kind:   model.GenerationKindImage,
	})
	assert.ErrorContains(t, err, "HTTPS")
}

func TestCleanupExpiredGenerationAssetsDeletesOneRecordedFile(t *testing.T) {
	configureGenerationAssetTest(t)
	asset, err := SaveGenerationAsset(GenerationAssetSaveRequest{
		UserID: 99,
		Role:   "input",
		Kind:   model.GenerationKindImage,
		Reader: bytes.NewReader(generationPNG(t)),
	})
	require.NoError(t, err)
	path, err := GenerationAssetPath(asset)
	require.NoError(t, err)

	expiredAt := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, model.DB.Model(&model.GenerationAsset{}).Where("id = ?", asset.ID).Update("expires_at", expiredAt).Error)
	deleted, err := CleanupExpiredGenerationAssets(time.Now().Unix(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	_, err = model.GetGenerationAssetForUser(asset.UserID, asset.PublicID)
	assert.True(t, model.IsRecordNotFound(err))
}

func TestInspectGenerationVideoRejectsImageBytes(t *testing.T) {
	configureGenerationAssetTest(t)
	_, err := SaveGenerationAsset(GenerationAssetSaveRequest{
		UserID: 10,
		Role:   "output",
		Kind:   model.GenerationKindVideo,
		Reader: bytes.NewReader(generationPNG(t)),
	})
	assert.True(t, errors.Is(err, ErrUnsupportedAssetType))
}
