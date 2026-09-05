package dto

const (
	MaxCreationReferenceImages = 4
	MaxCreationImageCount      = 4
	// Keep the aggregate input bound below the per-file maximum multiplied by
	// the number of files. Gemini and multipart image requests temporarily hold
	// encoded request data in memory, so this protects the gateway from a
	// single request amplifying memory usage.
	MaxCreationReferenceTotalBytes int64 = 40 * 1024 * 1024
)

type CreationImageRequest struct {
	Model             string   `json:"model"`
	Protocol          string   `json:"protocol"`
	Group             string   `json:"group,omitempty"`
	Prompt            string   `json:"prompt"`
	Size              string   `json:"size,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
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
