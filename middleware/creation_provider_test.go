package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreationProviderImageConversionUsesCompatibleJSON(t *testing.T) {
	originalRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios)) })
	for _, test := range []struct {
		model, protocol, size, expectedProtocol string
	}{
		{"无限制-Seedream-5-Pro", "seedream-image", "2048x2048", "seedream-image"},
		{"无限制-NovelAI-V5", "novelai-image", "832x1216", "novelai-image"},
		{"无限制-NovelAI-V4.5", "openai-image", "", "novelai-image"},
	} {
		t.Run(test.model, func(t *testing.T) {
			body, err := common.Marshal(dto.CreationImageRequest{
				Model: test.model, Protocol: test.protocol, Prompt: "A watercolor mountain landscape", Size: test.size,
			})
			require.NoError(t, err)
			router := gin.New()
			called := false
			router.POST("/pg/creation/images", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			}, CreationImageRequestConvert(), func(c *gin.Context) {
				called = true
				assert.Equal(t, "/v1/images/generations", c.Request.URL.Path)
				assert.Equal(t, gin.MIMEJSON, c.GetHeader("Content-Type"))
				request, ok := GetCreationImageRequest(c)
				require.True(t, ok)
				assert.Equal(t, test.expectedProtocol, request.Protocol)
				expectedSize := test.size
				if expectedSize == "" {
					expectedSize = "1024x1024"
				}
				assert.Equal(t, expectedSize, request.ResolvedSize)
				converted, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, common.Unmarshal(converted, &payload))
				assert.Equal(t, map[string]any{
					"model": test.model, "prompt": "A watercolor mountain landscape",
					"n": float64(1), "size": expectedSize, "response_format": "b64_json",
				}, payload)
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/pg/creation/images", bytes.NewReader(body))
			request.Header.Set("Content-Type", gin.MIMEJSON)
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
			assert.True(t, called)
		})
	}
}

func TestCreationProviderRejectsUnsupportedInputsBeforeRelay(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*dto.CreationImageRequest)
	}{
		{"multiple images", func(r *dto.CreationImageRequest) { r.Count = 2 }},
		{"negative count", func(r *dto.CreationImageRequest) { r.Count = -1 }},
		{"oversized dimensions", func(r *dto.CreationImageRequest) { r.Size = "99999x99999" }},
		{"reference image", func(r *dto.CreationImageRequest) { r.ReferenceAssetID = "another_users_asset" }},
		{"wrong model", func(r *dto.CreationImageRequest) { r.Model = "gpt-image-2" }},
		{"unsupported quality", func(r *dto.CreationImageRequest) { r.Quality = "high" }},
		{"conflicting aspect ratio", func(r *dto.CreationImageRequest) { r.AspectRatio = "16:9" }},
		{"native protocol not supported", func(r *dto.CreationImageRequest) { r.Protocol = "novelai-native" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := dto.CreationImageRequest{Model: "无限制-NovelAI-V5", Protocol: "novelai-image", Prompt: "A landscape", Count: 1}
			test.change(&payload)
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			router := gin.New()
			called := false
			router.POST("/pg/creation/images", CreationImageRequestConvert(), func(c *gin.Context) { called = true })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/pg/creation/images", bytes.NewReader(body))
			request.Header.Set("Content-Type", gin.MIMEJSON)
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.False(t, called, "invalid requests must not reach reservation or billing")
		})
	}
}
