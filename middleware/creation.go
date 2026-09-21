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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

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
		request.Group = strings.TrimSpace(request.Group)
		request.Prompt = strings.TrimSpace(request.Prompt)
		request.Size = strings.TrimSpace(request.Size)
		request.AspectRatio = strings.TrimSpace(request.AspectRatio)
		request.Quality = strings.TrimSpace(request.Quality)
		request.ReferenceAssetID = strings.TrimSpace(request.ReferenceAssetID)
		referenceAssetIDs, err := normalizeCreationReferenceAssetIDs(request.ReferenceAssetID, request.ReferenceAssetIDs)
		if err != nil {
			abortCreationRequest(c, err)
			return
		}
		request.ReferenceAssetIDs = referenceAssetIDs
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
		if request.Count < 1 || request.Count > dto.MaxCreationImageCount {
			abortCreationRequest(c, fmt.Errorf("count must be between 1 and %d", dto.MaxCreationImageCount))
			return
		}
		if request.AspectRatio != "" && !dto.IsCreationImageAspectRatioSupported(request.AspectRatio) {
			abortCreationRequest(c, errors.New("unsupported image aspect ratio"))
			return
		}
		if request.Protocol != "openai-image" && request.Protocol != "imagen" && request.Protocol != "gemini-image" {
			abortCreationRequest(c, errors.New("unsupported image protocol"))
			return
		}
		group, err := selectCreationGroup(c, request.Group)
		if err != nil {
			abortCreationRequestWithStatus(c, http.StatusForbidden, err)
			return
		}
		request.Group = group

		references := make([]*model.GenerationAsset, 0, len(request.ReferenceAssetIDs))
		if len(request.ReferenceAssetIDs) > 0 {
			if request.Protocol == "imagen" {
				abortCreationRequest(c, errors.New("the selected model does not support reference images"))
				return
			}
			var totalReferenceBytes int64
			for _, assetID := range request.ReferenceAssetIDs {
				asset, assetErr := model.GetGenerationAssetForUser(c.GetInt("id"), assetID)
				if assetErr != nil || !validCreationReferenceAsset(asset) {
					abortCreationRequest(c, errors.New("reference image not found or unavailable"))
					return
				}
				if asset.SizeBytes <= 0 || asset.SizeBytes > service.GenerationAssetLimit(model.GenerationKindImage) {
					abortCreationRequest(c, errors.New("reference image exceeds the per-file size limit"))
					return
				}
				totalReferenceBytes += asset.SizeBytes
				if totalReferenceBytes > dto.MaxCreationReferenceTotalBytes {
					abortCreationRequest(c, fmt.Errorf("reference images must be %d MB or smaller in total", dto.MaxCreationReferenceTotalBytes/(1<<20)))
					return
				}
				references = append(references, asset)
			}
		}

		var body []byte
		var contentType string
		var path string
		switch request.Protocol {
		case "gemini-image":
			path = "/v1beta/models/" + request.Model + ":generateContent"
			body, err = buildGeminiCreationImageBody(request, references)
			contentType = gin.MIMEJSON
		case "openai-image", "imagen":
			if len(references) > 0 {
				path = "/v1/images/edits"
				body, contentType, err = buildOpenAIImageEditBody(request, references)
			} else {
				path = "/v1/images/generations"
				body, err = buildOpenAIImageGenerationBody(request)
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
		request.Group = strings.TrimSpace(request.Group)
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
		group, err := selectCreationGroup(c, request.Group)
		if err != nil {
			abortCreationRequestWithStatus(c, http.StatusForbidden, err)
			return
		}
		request.Group = group

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

func normalizeCreationReferenceAssetIDs(legacyID string, requestedIDs []string) ([]string, error) {
	ids := make([]string, 0, len(requestedIDs)+1)
	seen := make(map[string]struct{}, len(requestedIDs)+1)
	for _, rawID := range requestedIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, errors.New("reference asset ids cannot be empty")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	legacyID = strings.TrimSpace(legacyID)
	if legacyID != "" {
		if _, exists := seen[legacyID]; !exists {
			ids = append(ids, legacyID)
		}
	}
	if len(ids) > dto.MaxCreationReferenceImages {
		return nil, fmt.Errorf("at most %d reference images are allowed", dto.MaxCreationReferenceImages)
	}
	return ids, nil
}

func validCreationReferenceAsset(asset *model.GenerationAsset) bool {
	if asset == nil || asset.Role != "input" || asset.JobID != 0 {
		return false
	}
	if asset.ExpiresAt > 0 && asset.ExpiresAt <= common.GetTimestamp() {
		return false
	}
	switch asset.MimeType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func buildGeminiCreationImageBody(request dto.CreationImageRequest, references []*model.GenerationAsset) ([]byte, error) {
	parts := []any{map[string]any{"text": request.Prompt}}
	for _, reference := range references {
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

func buildOpenAIImageGenerationBody(request dto.CreationImageRequest) ([]byte, error) {
	payload := map[string]any{
		"model":           request.Model,
		"prompt":          request.Prompt,
		"n":               request.Count,
		"response_format": "b64_json",
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.AspectRatio != "" {
		payload["aspect_ratio"] = request.AspectRatio
		if request.Size == "" {
			payload["size"] = request.AspectRatio
		}
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	return common.Marshal(payload)
}

func buildOpenAIImageEditBody(request dto.CreationImageRequest, references []*model.GenerationAsset) ([]byte, string, error) {
	if len(references) == 0 {
		return nil, "", errors.New("reference image is required")
	}
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
	if request.AspectRatio != "" {
		fields["aspect_ratio"] = request.AspectRatio
	}
	if request.Quality != "" {
		fields["quality"] = request.Quality
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	fieldName := "image"
	if len(references) > 1 {
		fieldName = "image[]"
	}
	for index, reference := range references {
		file, err := service.OpenGenerationAsset(reference)
		if err != nil {
			return nil, "", errors.New("reference image file not found")
		}
		header := make(textproto.MIMEHeader)
		filename := fmt.Sprintf("reference-%d%s", index+1, creationReferenceExtension(reference.MimeType))
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
		header.Set("Content-Type", reference.MimeType)
		part, createErr := writer.CreatePart(header)
		if createErr != nil {
			_ = file.Close()
			return nil, "", createErr
		}
		_, copyErr := io.Copy(part, io.LimitReader(file, service.GenerationAssetLimit(model.GenerationKindImage)+1))
		_ = file.Close()
		if copyErr != nil {
			return nil, "", copyErr
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func creationReferenceExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
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

func selectCreationGroup(c *gin.Context, requestedGroup string) (string, error) {
	accountGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	if accountGroup == "" {
		accountGroup = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	}
	if accountGroup == "" {
		return "", errors.New("user group is not available")
	}

	group := strings.TrimSpace(requestedGroup)
	if group == "" {
		group = accountGroup
	}
	if len(group) > 64 || strings.ContainsAny(group, ",\x00") {
		return "", errors.New("invalid channel group")
	}
	if group != accountGroup && !service.GroupInUserUsableGroups(accountGroup, group) {
		return "", fmt.Errorf("no access to channel group %s", group)
	}
	if group == "auto" || !ratio_setting.ContainsGroupRatio(group) {
		return "", fmt.Errorf("channel group %s is not active", group)
	}

	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	return group, nil
}

func abortCreationRequest(c *gin.Context, err error) {
	abortCreationRequestWithStatus(c, http.StatusBadRequest, err)
}

func abortCreationRequestWithStatus(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{
		"success": false,
		"message": err.Error(),
	})
	c.Abort()
}
