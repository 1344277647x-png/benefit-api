package middleware

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCreationReferenceAssetIDsPreservesOrderAndLegacyCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		legacyID  string
		ids       []string
		expected  []string
		errorText string
	}{
		{
			name:     "legacy field becomes one array entry",
			legacyID: "asset_legacy",
			expected: []string{"asset_legacy"},
		},
		{
			name:     "array order is stable and duplicates are removed",
			ids:      []string{"asset_b", "asset_a", "asset_b", "asset_c"},
			expected: []string{"asset_b", "asset_a", "asset_c"},
		},
		{
			name:     "legacy field is appended when it is new",
			legacyID: "asset_legacy",
			ids:      []string{"asset_a", "asset_b"},
			expected: []string{"asset_a", "asset_b", "asset_legacy"},
		},
		{
			name:      "explicit empty array entry is rejected",
			ids:       []string{"asset_a", " "},
			errorText: "cannot be empty",
		},
		{
			name: "more than four unique references are rejected",
			ids: []string{
				"asset_a",
				"asset_b",
				"asset_c",
				"asset_d",
				"asset_e",
			},
			errorText: "at most 4",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := normalizeCreationReferenceAssetIDs(test.legacyID, test.ids)
			if test.errorText != "" {
				require.ErrorContains(t, err, test.errorText)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestValidCreationReferenceAssetRejectsUnavailableAssets(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name  string
		asset *model.GenerationAsset
		valid bool
	}{
		{name: "ready png", asset: &model.GenerationAsset{Role: "input", Status: "ready", MimeType: "image/png", ExpiresAt: now + 60}, valid: true},
		{name: "bound asset", asset: &model.GenerationAsset{Role: "input", Status: "ready", MimeType: "image/png", JobID: 9, ExpiresAt: now + 60}},
		{name: "expired asset", asset: &model.GenerationAsset{Role: "input", Status: "ready", MimeType: "image/png", ExpiresAt: now - 1}},
		{name: "svg asset", asset: &model.GenerationAsset{Role: "input", Status: "ready", MimeType: "image/svg+xml", ExpiresAt: now + 60}},
		{name: "output asset", asset: &model.GenerationAsset{Role: "output", Status: "ready", MimeType: "image/png", ExpiresAt: now + 60}},
		{name: "missing asset", asset: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.valid, validCreationReferenceAsset(test.asset))
		})
	}
}

func TestBuildOpenAIImageEditBodyIncludesAllReferencesInOrder(t *testing.T) {
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "openai-multi-reference")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	paths := []string{"reference-a.png", "reference-b.jpg", "reference-c.webp"}
	contents := [][]byte{[]byte("first"), []byte("second"), []byte("third")}
	for index, path := range paths {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), contents[index], 0o600))
		path := path
		t.Cleanup(func() { _ = os.Remove(filepath.Join(root, path)) })
	}

	references := []*model.GenerationAsset{
		{RelativePath: paths[0], MimeType: "image/png"},
		{RelativePath: paths[1], MimeType: "image/jpeg"},
		{RelativePath: paths[2], MimeType: "image/webp"},
	}
	body, contentType, err := buildOpenAIImageEditBody(dto.CreationImageRequest{
		Model:       "gpt-image-2",
		Prompt:      "keep the product consistent",
		Count:       dto.MaxCreationImageCount,
		AspectRatio: "21:9",
	}, references)
	require.NoError(t, err)

	_, parameters, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), parameters["boundary"])
	var imageParts [][]byte
	var filenames []string
	fields := map[string]string{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		if part.FileName() == "" {
			fields[part.FormName()] = string(data)
			continue
		}
		assert.Equal(t, "image[]", part.FormName())
		imageParts = append(imageParts, data)
		filenames = append(filenames, part.FileName())
	}
	assert.Equal(t, contents, imageParts)
	assert.Equal(t, []string{"reference-1.png", "reference-2.jpg", "reference-3.webp"}, filenames)
	assert.Equal(t, "4", fields["n"])
	assert.Equal(t, "21:9", fields["aspect_ratio"])
}

func TestBuildOpenAIImageGenerationBodyPreservesSizeAndAspectRatio(t *testing.T) {
	body, err := buildOpenAIImageGenerationBody(dto.CreationImageRequest{
		Model:       "gpt-image-2",
		Prompt:      "cinematic landscape",
		Size:        "1536x1024",
		AspectRatio: "3:2",
		Quality:     "high",
		Count:       2,
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "1536x1024", payload["size"])
	assert.Equal(t, "3:2", payload["aspect_ratio"])
	assert.Equal(t, "high", payload["quality"])
	assert.Equal(t, float64(2), payload["n"])
}

func TestBuildGeminiCreationImageBodyIncludesAllInlineData(t *testing.T) {
	root := filepath.Join("D:\\照片\\OneDrive\\桌面\\api\\.cache\\go-tests", "gemini-multi-reference")
	require.NoError(t, os.MkdirAll(root, 0o750))
	t.Setenv("GENERATION_ASSET_ROOT", root)
	t.Cleanup(func() { _ = os.Remove(root) })
	paths := []string{"gemini-a.png", "gemini-b.jpg"}
	contents := [][]byte{[]byte("alpha"), []byte("beta")}
	for index, path := range paths {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), contents[index], 0o600))
		path := path
		t.Cleanup(func() { _ = os.Remove(filepath.Join(root, path)) })
	}

	body, err := buildGeminiCreationImageBody(dto.CreationImageRequest{
		Prompt:      "make a consistent set",
		Count:       dto.MaxCreationImageCount,
		AspectRatio: "2:3",
	}, []*model.GenerationAsset{
		{RelativePath: paths[0], MimeType: "image/png"},
		{RelativePath: paths[1], MimeType: "image/jpeg"},
	})
	require.NoError(t, err)
	var payload struct {
		Contents []struct {
			Parts []struct {
				Text       string `json:"text"`
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			CandidateCount int `json:"candidateCount"`
			ImageConfig    struct {
				AspectRatio string `json:"aspectRatio"`
			} `json:"imageConfig"`
		} `json:"generationConfig"`
	}
	require.NoError(t, common.Unmarshal(body, &payload))
	require.Len(t, payload.Contents, 1)
	require.Len(t, payload.Contents[0].Parts, 3)
	assert.Equal(t, "make a consistent set", payload.Contents[0].Parts[0].Text)
	assert.Equal(t, "image/png", payload.Contents[0].Parts[1].InlineData.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(contents[0]), payload.Contents[0].Parts[1].InlineData.Data)
	assert.Equal(t, "image/jpeg", payload.Contents[0].Parts[2].InlineData.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(contents[1]), payload.Contents[0].Parts[2].InlineData.Data)
	assert.Equal(t, dto.MaxCreationImageCount, payload.GenerationConfig.CandidateCount)
	assert.Equal(t, "2:3", payload.GenerationConfig.ImageConfig.AspectRatio)
}

func TestCreationImageAspectRatiosMatchCreationCenterOptions(t *testing.T) {
	expected := []string{"1:1", "3:2", "2:3", "16:9", "9:16", "4:3", "3:4", "21:9"}
	assert.Equal(t, expected, dto.CreationImageAspectRatios())
	for _, ratio := range expected {
		assert.True(t, dto.IsCreationImageAspectRatioSupported(ratio), ratio)
	}
	assert.False(t, dto.IsCreationImageAspectRatioSupported("4:1"))
}
