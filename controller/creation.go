package controller

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/creation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type creationModelCapabilities struct {
	ReferenceImage bool     `json:"reference_image"`
	MaxCount       int      `json:"max_count,omitempty"`
	Sizes          []string `json:"sizes,omitempty"`
	AspectRatios   []string `json:"aspect_ratios,omitempty"`
	Qualities      []string `json:"qualities,omitempty"`
	Durations      []int    `json:"durations,omitempty"`
	Resolutions    []string `json:"resolutions,omitempty"`
}

type creationModel struct {
	ID           string                    `json:"id"`
	DisplayName  string                    `json:"display_name"`
	Kind         string                    `json:"kind"`
	Protocol     string                    `json:"protocol"`
	Groups       []string                  `json:"groups"`
	Capabilities creationModelCapabilities `json:"capabilities"`
}

func requireCreationEnabled(c *gin.Context) bool {
	if creation_setting.Enabled() {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"message": "AI creation center is not enabled",
	})
	return false
}

func GetCreationModels(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	models, err := creationModelsForUser(user)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usage, err := model.GetGenerationStorageUsage(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"models":         models,
		"storage":        usage,
		"retention_days": creation_setting.GetSetting().RetentionDays,
	})
}

func creationModelsForUser(user *model.UserBase) ([]creationModel, error) {
	if user == nil {
		return []creationModel{}, nil
	}
	usableGroups := service.GetUserUsableGroups(user.Group)
	endpointGroups, err := model.GetEnabledModelEndpointGroups()
	if err != nil {
		return nil, err
	}
	models := make([]creationModel, 0)
	for _, pricing := range model.GetPricing() {
		modelEndpointGroups := endpointGroups[pricing.ModelName]
		geminiGroups := compatibleCreationGroups(
			modelEndpointGroups[constant.EndpointTypeGeminiImage],
			pricing.EnableGroup,
			usableGroups,
			user.Group,
		)
		imageGroups := compatibleCreationGroups(
			modelEndpointGroups[constant.EndpointTypeImageGeneration],
			pricing.EnableGroup,
			usableGroups,
			user.Group,
		)
		videoGroups := compatibleCreationGroups(
			modelEndpointGroups[constant.EndpointTypeOpenAIVideo],
			pricing.EnableGroup,
			usableGroups,
			user.Group,
		)

		if len(geminiGroups) > 0 {
			models = append(models, creationModel{
				ID:          pricing.ModelName,
				DisplayName: pricing.ModelName,
				Kind:        model.GenerationKindImage,
				Protocol:    "gemini-image",
				Groups:      geminiGroups,
				Capabilities: creationModelCapabilities{
					ReferenceImage: true,
					MaxCount:       4,
					AspectRatios:   []string{"1:1", "16:9", "9:16", "4:3", "3:4"},
				},
			})
		} else if len(imageGroups) > 0 {
			protocol := "openai-image"
			capabilities := creationModelCapabilities{
				ReferenceImage: true,
				MaxCount:       4,
				Sizes:          []string{"1024x1024", "1536x1024", "1024x1536"},
				Qualities:      []string{"auto", "low", "medium", "high"},
			}
			if strings.HasPrefix(strings.ToLower(pricing.ModelName), "imagen-") {
				protocol = "imagen"
				capabilities.ReferenceImage = false
				capabilities.Sizes = nil
				capabilities.Qualities = nil
				capabilities.AspectRatios = []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
			}
			models = append(models, creationModel{
				ID:           pricing.ModelName,
				DisplayName:  pricing.ModelName,
				Kind:         model.GenerationKindImage,
				Protocol:     protocol,
				Groups:       imageGroups,
				Capabilities: capabilities,
			})
		}
		if len(videoGroups) > 0 {
			models = append(models, creationModel{
				ID:          pricing.ModelName,
				DisplayName: pricing.ModelName,
				Kind:        model.GenerationKindVideo,
				Protocol:    "openai-video",
				Groups:      videoGroups,
				Capabilities: creationModelCapabilities{
					ReferenceImage: true,
					Durations:      []int{4, 5, 6, 8, 10},
					Resolutions:    []string{"720p", "1080p"},
				},
			})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Kind == models[j].Kind {
			return models[i].ID < models[j].ID
		}
		return models[i].Kind < models[j].Kind
	})
	return models, nil
}

func compatibleCreationGroups(
	endpointGroups []string,
	pricingGroups []string,
	usableGroups map[string]string,
	accountGroup string,
) []string {
	pricingAllowsAll := common.StringsContains(pricingGroups, "all")
	pricingGroupSet := make(map[string]struct{}, len(pricingGroups))
	for _, group := range pricingGroups {
		pricingGroupSet[group] = struct{}{}
	}

	seen := make(map[string]struct{}, len(endpointGroups))
	groups := make([]string, 0, len(endpointGroups))
	for _, group := range endpointGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := usableGroups[group]; !ok {
			continue
		}
		if !ratio_setting.ContainsGroupRatio(group) {
			continue
		}
		if !pricingAllowsAll {
			if _, ok := pricingGroupSet[group]; !ok {
				continue
			}
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i] == accountGroup {
			return true
		}
		if groups[j] == accountGroup {
			return false
		}
		return groups[i] < groups[j]
	})
	return groups
}

func UploadCreationAsset(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	limit := service.GenerationAssetLimit(model.GenerationKindImage)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+(1<<20))
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "multipart image upload is required"})
		return
	}

	var asset *model.GenerationAsset
	for {
		part, nextErr := reader.NextPart()
		if nextErr != nil {
			if errors.Is(nextErr, io.EOF) {
				break
			}
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": nextErr.Error()})
			return
		}
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}
		if asset != nil {
			_ = part.Close()
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "only one reference image is allowed"})
			return
		}
		asset, err = service.SaveGenerationAsset(service.GenerationAssetSaveRequest{
			UserID:   c.GetInt("id"),
			Role:     "input",
			Kind:     model.GenerationKindImage,
			Reader:   part,
			MaxBytes: limit,
		})
		_ = part.Close()
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, service.ErrGenerationAssetTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			c.JSON(status, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	if asset == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "reference image is required"})
		return
	}
	decorateCreationAsset(asset)
	common.ApiSuccess(c, asset)
}

func ListCreationJobs(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	pageInfo := common.GetPageQuery(c)
	jobs, total, err := model.ListGenerationJobs(c.GetInt("id"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, job := range jobs {
		decorateCreationJob(job)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(jobs)
	common.ApiSuccess(c, pageInfo)
}

func GetCreationJob(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	job, err := model.GetGenerationJobForUser(c.GetInt("id"), c.Param("id"))
	if err != nil {
		if model.IsRecordNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "generation job not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	decorateCreationJob(job)
	common.ApiSuccess(c, job)
}

func GetCreationAssetContent(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	asset, err := model.GetGenerationAssetForUser(c.GetInt("id"), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "generation asset not found"})
		return
	}
	if asset.ExpiresAt > 0 && asset.ExpiresAt <= time.Now().Unix() {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "generation asset has expired"})
		return
	}
	file, err := service.OpenGenerationAsset(asset)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "generation asset file not found"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", asset.MimeType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=3600")
	disposition := "inline"
	if c.Query("download") == "1" {
		disposition = "attachment"
	}
	extension := extensionForMimeType(asset.MimeType)
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{
		"filename": asset.PublicID + extension,
	}))
	http.ServeContent(c.Writer, c.Request, asset.PublicID+extension, info.ModTime(), file)
}

func DeleteCreationJob(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	job, assets, err := model.DeleteGenerationJobForUser(c.GetInt("id"), c.Param("id"))
	if err != nil {
		status := http.StatusBadRequest
		if model.IsRecordNotFound(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	for i := range assets {
		asset := &assets[i]
		if err := service.RemoveGenerationAssetFile(asset); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := model.DeleteGenerationAssetRecord(asset.ID); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if err := model.DeleteGenerationJobRecord(job.ID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": job.PublicID})
}

func RetryCreationArchive(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	job, err := model.GetGenerationJobForUser(c.GetInt("id"), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "generation job not found"})
		return
	}
	if job.Kind != model.GenerationKindVideo || job.Status != model.GenerationJobArchiveFailed {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "generation job is not ready for archive retry"})
		return
	}
	if err := model.UpdateGenerationJob(job.ID, map[string]any{
		"status":           model.GenerationJobArchiving,
		"archive_attempts": 0,
		"next_archive_at":  time.Now().Unix(),
		"error_code":       "",
		"error_message":    "",
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	job, err = model.GetGenerationJobForUser(c.GetInt("id"), c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	decorateCreationJob(job)
	common.ApiSuccess(c, job)
}

func decorateCreationJob(job *model.GenerationJob) {
	if job == nil {
		return
	}
	for i := range job.Assets {
		decorateCreationAsset(&job.Assets[i])
	}
}

func decorateCreationAsset(asset *model.GenerationAsset) {
	if asset == nil {
		return
	}
	asset.ContentURL = "/api/creation/assets/" + asset.PublicID + "/content"
}

func extensionForMimeType(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	default:
		return ""
	}
}

func creationErrorCode(status int) string {
	return "creation_" + strconv.Itoa(status)
}

func creationErrorMessage(body []byte, fallback string) string {
	message := extractCreationErrorMessage(body)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		return fallback
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	return fmt.Sprintf("%s: %s", fallback, message)
}

func extractCreationErrorMessage(body []byte) string {
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	if value, ok := payload["error"]; ok {
		if message, ok := value.(string); ok && strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message)
		}
		if object, ok := value.(map[string]any); ok {
			if message, ok := object["message"].(string); ok && strings.TrimSpace(message) != "" {
				return strings.TrimSpace(message)
			}
		}
	}
	return ""
}
