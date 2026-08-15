package dto

type CreationImageRequest struct {
	Model            string `json:"model"`
	Protocol         string `json:"protocol"`
	Group            string `json:"group,omitempty"`
	Prompt           string `json:"prompt"`
	Size             string `json:"size,omitempty"`
	AspectRatio      string `json:"aspect_ratio,omitempty"`
	Quality          string `json:"quality,omitempty"`
	Count            int    `json:"count,omitempty"`
	ReferenceAssetID string `json:"reference_asset_id,omitempty"`
}

type CreationVideoRequest struct {
	Model            string `json:"model"`
	Group            string `json:"group,omitempty"`
	Prompt           string `json:"prompt"`
	Duration         int    `json:"duration"`
	Resolution       string `json:"resolution"`
	ReferenceAssetID string `json:"reference_asset_id,omitempty"`
}
