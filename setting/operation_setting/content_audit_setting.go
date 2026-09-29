package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const ContentAuditRetentionDays = 7

type ContentAuditSetting struct {
	Enabled      bool `json:"enabled"`
	PrivacyReady bool `json:"privacy_ready"`
}

var contentAuditSetting = ContentAuditSetting{}

func init() {
	config.GlobalConfig.Register("content_audit", &contentAuditSetting)
}

func GetContentAuditSettingSnapshot() ContentAuditSetting { return contentAuditSetting }
