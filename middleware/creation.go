package middleware

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	creationImageRequestKey = "creation_image_request"
	creationVideoRequestKey = "creation_video_request"
)

func GetCreationImageRequest(c *gin.Context) (dto.CreationImageRequest, bool) {
	request, ok := c.Get(creationImageRequestKey)
	if !ok {
		return dto.CreationImageRequest{}, false
	}
	value, ok := request.(dto.CreationImageRequest)
	return value, ok
}

func GetCreationVideoRequest(c *gin.Context) (dto.CreationVideoRequest, bool) {
	request, ok := c.Get(creationVideoRequestKey)
	if !ok {
		return dto.CreationVideoRequest{}, false
	}
	value, ok := request.(dto.CreationVideoRequest)
	return value, ok
}

func CreationImageRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request dto.CreationImageRequest
		if err := decodeCreationRequest(c, &request); err != nil {
			abortCreationRequest(c, err)
			return
		}
		request.Model = strings.TrimSpace(request.Model)
		request.Protocol = strings.TrimSpace(request.Protocol)
		request.Prompt = strings.TrimSpace(request.Prompt)
		request.Size = strings.TrimSpace(request.Size)
		request.AspectRatio = strings.TrimSpace(request.AspectRatio)
		request.Quality = strings.TrimSpace(request.Quality)
		request.ReferenceAssetID = strings.TrimSpace(request.ReferenceAssetID)
		if request.Model == "" || request.Prompt == "" {
			abortCreationRequest(c, errors.New("model and prompt are required"))
			return
		}
		if len(request.Model) > 191 || strings.ContainsAny(request.Model, "/?#\\\x00") {
			abortCreationRequest(c, errors.New("invalid model name"))
			return
		}
		if len(request.Prompt) > 20000 {
			abortCreationRequest(c, errors.New("prompt is too long"))
			return
		}
		if request.Count == 0 {
			request.Count = 1
		}
		if request.Count < 1 || request.Count > 4 {
			abortCreationRequest(c, errors.New("count must be between 1 and 4"))
			return
		}
		if request.Protocol != "openai-image" && request.Protocol != "imagen" && request.Protocol != "gemini-image" {
			abortCreationRequest(c, errors.New("unsupported image protocol"))
			return
		}

		var reference *model.GenerationAsset
		if request.ReferenceAssetID != "" {
			if request.Protocol == "imagen" {
				abortCreationRequest(c, errors.New("the selected model does not support a reference image"))
				return
			}
			asset, err := model.GetGenerationAssetForUser(c.GetInt("id"), request.ReferenceAssetID)
			if err != nil || asset.Role != "input" || !strings.HasPrefix(asset.MimeType, "image/") {
				abortCreationRequest(c, errors.New("reference image not found"))
				return
			}
			reference = asset
		}

		var body []byte
		var contentType string
		var path string
		var err error
		switch request.Protocol {
		case "gemini-image":
			path = "/v1beta/models/" + request.Model + ":generateContent"
			body, err = buildGeminiCreationImageBody(request, reference)
			contentType = gin.MIMEJSON
		case "openai-image", "imagen":
			if reference != nil {
				path = "/v1/images/edits"
				body, contentType, err = buildOpenAIImageEditBody(request, reference)
			} else {
				path = "/v1/images/generations"
				payload := map[string]any{
					"model":           request.Model,
					"prompt":          request.Prompt,
					"n":               request.Count,
					"response_format": "b64_json",
				}
				if request.Size != "" {
					payload["size"] = request.Size
				} else if request.AspectRatio != "" {
					payload["size"] = request.AspectRatio
					payload["aspect_ratio"] = request.AspectRatio
				}
				if request.Quality != "" {
					payload["quality"] = request.Quality
				}
				body, err = common.Marshal(payload)
				contentType = gin.MIMEJSON
			}
		}
		if err != nil {
			abortCreationRequest(c, err)
			return
		}
		c.Set(creationImageRequestKey, request)
		setCreationRelayRequest(c, path, contentType, body)
		c.Next()
	}
}

func CreationVideoRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		var request dto.CreationVideoRequest
		if err := decodeCreationRequest(c, &request); err != nil {
			abortCreationRequest(c, err)
			return
		}
		request.Model = strings.TrimSpace(request.Model)
		request.Prompt = strings.TrimSpace(request.Prompt)
		request.Resolution = strings.TrimSpace(request.Resolution)
		request.ReferenceAssetID = strings.TrimSpace(request.ReferenceAssetID)
		if request.Model == "" || request.Prompt == "" {
			abortCreationRequest(c, errors.New("model and prompt are required"))
			return
		}
		if len(request.Model) > 191 || strings.ContainsAny(request.Model, "/?#\\\x00") {
			abortCreationRequest(c, errors.New("invalid model name"))
			return
		}
		if len(request.Prompt) > 20000 {
			abortCreationRequest(c, errors.New("prompt is too long"))
			return
		}
		if request.Duration <= 0 || request.Duration > relaycommon.MaxTaskDurationSeconds {
			abortCreationRequest(c, fmt.Errorf("duration must be between 1 and %d seconds", relaycommon.MaxTaskDurationSeconds))
			return
		}

		payload := map[string]any{
			"model":    request.Model,
			"prompt":   request.Prompt,
			"duration": request.Duration,
			"seconds":  strconv.Itoa(request.Duration),
		}
		if request.Resolution != "" {
			payload["size"] = creationVideoSize(request.Resolution)
			payload["resolution"] = request.Resolution
		}
		if request.ReferenceAssetID != "" {
			asset, err := model.GetGenerationAssetForUser(c.GetInt("id"), request.ReferenceAssetID)
			if err != nil || asset.Role != "input" || !strings.HasPrefix(asset.MimeType, "image/") {
				abortCreationRequest(c, errors.New("reference image not found"))
				return
			}
			file, err := service.OpenGenerationAsset(asset)
			if err != nil {
				abortCreationRequest(c, errors.New("reference image file not found"))
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(file, service.GenerationAssetLimit(model.GenerationKindImage)+1))
			_ = file.Close()
			if readErr != nil || int64(len(data)) > service.GenerationAssetLimit(model.GenerationKindImage) {
				abortCreationRequest(c, errors.New("reference image cannot be read"))
				return
			}
			payload["input_reference"] = "data:" + asset.MimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
		body, err := common.Marshal(payload)
		if err != nil {
			abortCreationRequest(c, err)
			return
		}
		c.Set(creationVideoRequestKey, request)
		setCreationRelayRequest(c, "/v1/videos", gin.MIMEJSON, body)
		c.Next()
	}
}

func decodeCreationRequest(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if err := common.DecodeJson(c.Request.Body, target); err != nil {
		return fmt.Errorf("invalid creation request: %w", err)
	}
	return nil
}

func setCreationRelayRequest(c *gin.Context, path string, contentType string, body []byte) {
	common.CleanupBodyStorage(c)
	c.Request.URL.Path = path
	c.Request.URL.RawPath = ""
	c.Request.RequestURI = path
	c.Request.Header.Set("Content-Type", contentType)
	c.Request.ContentLength = int64(len(body))
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Set("is_playground", true)
}

func buildGeminiCreationImageBody(request dto.CreationImageRequest, reference *model.GenerationAsset) ([]byte, error) {
	parts := []any{map[string]any{"text": request.Prompt}}
	if reference != nil {
		file, err := service.OpenGenerationAsset(reference)
		if err != nil {
			return nil, errors.New("reference image file not found")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, service.GenerationAssetLimit(model.GenerationKindImage)+1))
		_ = file.Close()
		if readErr != nil || int64(len(data)) > service.GenerationAssetLimit(model.GenerationKindImage) {
			return nil, errors.New("reference image cannot be read")
		}
		parts = append(parts, map[string]any{
			"inlineData": map[string]any{
				"mimeType": reference.MimeType,
				"data":     base64.StdEncoding.EncodeToString(data),
			},
		})
	}
	imageConfig := map[string]any{}
	if request.AspectRatio != "" {
		imageConfig["aspectRatio"] = request.AspectRatio
	}
	generationConfig := map[string]any{
		"responseModalities": []string{"IMAGE"},
	}
	// Gemini's native generateContent API uses candidateCount for multiple
	// image candidates; keeping this in generationConfig preserves the relay
	// request shape while honoring the creation center's 1-4 image control.
	if request.Count > 1 {
		generationConfig["candidateCount"] = request.Count
	}
	if len(imageConfig) > 0 {
		generationConfig["imageConfig"] = imageConfig
	}
	return common.Marshal(map[string]any{
		"contents": []any{map[string]any{
			"role":  "user",
			"parts": parts,
		}},
		"generationConfig": generationConfig,
	})
}

func buildOpenAIImageEditBody(request dto.CreationImageRequest, reference *model.GenerationAsset) ([]byte, string, error) {
	file, err := service.OpenGenerationAsset(reference)
	if err != nil {
		return nil, "", errors.New("reference image file not found")
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":           request.Model,
		"prompt":          request.Prompt,
		"n":               strconv.Itoa(request.Count),
		"response_format": "b64_json",
	}
	if request.Size != "" {
		fields["size"] = request.Size
	}
	if request.Quality != "" {
		fields["quality"] = request.Quality
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="image"; filename="reference"`)
	header.Set("Content-Type", reference.MimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, io.LimitReader(file, service.GenerationAssetLimit(model.GenerationKindImage)+1)); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func creationVideoSize(resolution string) string {
	switch resolution {
	case "1080p":
		return "1920x1080"
	case "720p":
		return "1280x720"
	default:
		return resolution
	}
}

func abortCreationRequest(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"message": err.Error(),
	})
	c.Abort()
}
