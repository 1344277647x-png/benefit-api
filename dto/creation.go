package dto

import "strings"

const (
	MaxCreationReferenceImages     = 4
	MaxCreationImageCount          = 4
	DefaultCreationImageResolution = "4K"
	// Keep the aggregate input bound below the per-file maximum multiplied by
	// the number of files. Gemini and multipart image requests temporarily hold
	// encoded request data in memory, so this protects the gateway from a
	// single request amplifying memory usage.
	MaxCreationReferenceTotalBytes int64 = 40 * 1024 * 1024
)

var creationImageResolutionTiers = []string{"1K", "2K", "4K"}

var creationImageSizePresets = map[string]map[string]string{
	"1K": {
		"1:1": "1024x1024", "3:2": "1536x1024", "2:3": "1024x1536", "16:9": "1280x720",
		"9:16": "720x1280", "4:3": "1024x768", "3:4": "768x1024", "21:9": "1280x544",
	},
	"2K": {
		"1:1": "2048x2048", "3:2": "2160x1440", "2:3": "1440x2160", "16:9": "2560x1440",
		"9:16": "1440x2560", "4:3": "2048x1536", "3:4": "1536x2048", "21:9": "2560x1088",
	},
	"4K": {
		"1:1": "2880x2880", "3:2": "3456x2304", "2:3": "2304x3456", "16:9": "3840x2160",
		"9:16": "2160x3840", "4:3": "3200x2400", "3:4": "2400x3200", "21:9": "3840x1600",
	},
}

func IsImage2Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "gpt-image-2" || strings.HasPrefix(model, "gpt-image-2.")
}

func CreationImageResolutionTiers() []string {
	return append([]string(nil), creationImageResolutionTiers...)
}

func CreationImageSizePresets() map[string]map[string]string {
	presets := make(map[string]map[string]string, len(creationImageSizePresets))
	for resolution, sizes := range creationImageSizePresets {
		presets[resolution] = make(map[string]string, len(sizes))
		for aspectRatio, size := range sizes {
			presets[resolution][aspectRatio] = size
		}
	}
	return presets
}

func ResolveImage2Size(resolution string, aspectRatio string) (string, bool) {
	sizes, ok := creationImageSizePresets[strings.ToUpper(strings.TrimSpace(resolution))]
	if !ok {
		return "", false
	}
	size, ok := sizes[strings.TrimSpace(aspectRatio)]
	return size, ok
}

func CreationImageAspectRatios() []string {
	return []string{"1:1", "3:2", "2:3", "16:9", "9:16", "4:3", "3:4", "21:9"}
}

func IsCreationImageAspectRatioSupported(value string) bool {
	switch value {
	case "1:1", "3:2", "2:3", "16:9", "9:16", "4:3", "3:4", "21:9":
		return true
	default:
		return false
	}
}

type CreationImageRequest struct {
	Model             string   `json:"model"`
	Protocol          string   `json:"protocol"`
	Group             string   `json:"group,omitempty"`
	Prompt            string   `json:"prompt"`
	Size              string   `json:"size,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	ResolvedSize      string   `json:"resolved_size,omitempty"`
	Quality           string   `json:"quality,omitempty"`
	Count             int      `json:"count,omitempty"`
	ReferenceAssetID  string   `json:"reference_asset_id,omitempty"`
	ReferenceAssetIDs []string `json:"reference_asset_ids,omitempty"`
}

type CreationVideoRequest struct {
	Model            string `json:"model"`
	Group            string `json:"group,omitempty"`
	Prompt           string `json:"prompt"`
	Duration         int    `json:"duration"`
	Resolution       string `json:"resolution"`
	ReferenceAssetID string `json:"reference_asset_id,omitempty"`
}
