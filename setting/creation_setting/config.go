package creation_setting

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type CreationSetting struct {
	Enabled             bool `json:"enabled"`
	RetentionDays       int  `json:"retention_days"`
	MaxImageMB          int  `json:"max_image_mb"`
	MaxVideoMB          int  `json:"max_video_mb"`
	MaxUserStorageMB    int  `json:"max_user_storage_mb"`
	MaxSystemStorageMB  int  `json:"max_system_storage_mb"`
	MaxPendingVideoJobs int  `json:"max_pending_video_jobs"`
}

var creationSetting = CreationSetting{
	Enabled:             false,
	RetentionDays:       7,
	MaxImageMB:          20,
	MaxVideoMB:          500,
	MaxUserStorageMB:    1024,
	MaxSystemStorageMB:  10 * 1024,
	MaxPendingVideoJobs: 2,
}

func init() {
	config.GlobalConfig.Register("creation_setting", &creationSetting)
}

func GetSetting() CreationSetting {
	setting := creationSetting
	if setting.RetentionDays < 1 {
		setting.RetentionDays = 1
	} else if setting.RetentionDays > 30 {
		setting.RetentionDays = 30
	}
	if setting.MaxImageMB < 1 {
		setting.MaxImageMB = 1
	} else if setting.MaxImageMB > 20 {
		setting.MaxImageMB = 20
	}
	if setting.MaxVideoMB < 1 {
		setting.MaxVideoMB = 1
	} else if setting.MaxVideoMB > 500 {
		setting.MaxVideoMB = 500
	}
	if setting.MaxUserStorageMB < setting.MaxImageMB {
		setting.MaxUserStorageMB = setting.MaxImageMB
	} else if setting.MaxUserStorageMB > 1024 {
		setting.MaxUserStorageMB = 1024
	}
	if setting.MaxSystemStorageMB < setting.MaxUserStorageMB {
		setting.MaxSystemStorageMB = setting.MaxUserStorageMB
	} else if setting.MaxSystemStorageMB > 10*1024 {
		setting.MaxSystemStorageMB = 10 * 1024
	}
	if setting.MaxPendingVideoJobs < 1 {
		setting.MaxPendingVideoJobs = 1
	} else if setting.MaxPendingVideoJobs > 2 {
		setting.MaxPendingVideoJobs = 2
	}
	return setting
}

func Enabled() bool {
	value := strings.TrimSpace(os.Getenv("CREATION_ENABLED"))
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	return creationSetting.Enabled
}

func AssetRoot() string {
	if value := strings.TrimSpace(os.Getenv("GENERATION_ASSET_ROOT")); value != "" {
		return value
	}
	return "/data/generation-assets"
}
