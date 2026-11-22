package channel_health_setting

import "github.com/QuantumNous/new-api/setting/config"

type ChannelHealthSetting struct {
	Enabled                      bool `json:"enabled"`
	RefreshIntervalSeconds       int  `json:"refresh_interval_seconds"`
	WindowMinutes                int  `json:"window_minutes"`
	DelayedThresholdMilliseconds int  `json:"delayed_threshold_ms"`
	FailureStreakThreshold       int  `json:"failure_streak_threshold"`
	MinimumSamples               int  `json:"minimum_samples"`
	UnavailableErrorRatePercent  int  `json:"unavailable_error_rate_percent"`
	StaleAfterMinutes            int  `json:"stale_after_minutes"`
	RetentionDays                int  `json:"retention_days"`
}

var channelHealthSetting = ChannelHealthSetting{
	Enabled:                      true,
	RefreshIntervalSeconds:       15,
	WindowMinutes:                5,
	DelayedThresholdMilliseconds: 10_000,
	FailureStreakThreshold:       5,
	MinimumSamples:               5,
	UnavailableErrorRatePercent:  70,
	StaleAfterMinutes:            15,
	RetentionDays:                7,
}

func init() {
	config.GlobalConfig.Register("channel_health_setting", &channelHealthSetting)
}

func GetSetting() ChannelHealthSetting {
	setting := channelHealthSetting
	if setting.RefreshIntervalSeconds < 5 {
		setting.RefreshIntervalSeconds = 5
	} else if setting.RefreshIntervalSeconds > 300 {
		setting.RefreshIntervalSeconds = 300
	}
	if setting.WindowMinutes < 1 {
		setting.WindowMinutes = 1
	} else if setting.WindowMinutes > 60 {
		setting.WindowMinutes = 60
	}
	if setting.DelayedThresholdMilliseconds < 100 {
		setting.DelayedThresholdMilliseconds = 100
	} else if setting.DelayedThresholdMilliseconds > 3_600_000 {
		setting.DelayedThresholdMilliseconds = 3_600_000
	}
	if setting.FailureStreakThreshold < 1 {
		setting.FailureStreakThreshold = 1
	} else if setting.FailureStreakThreshold > 100 {
		setting.FailureStreakThreshold = 100
	}
	if setting.MinimumSamples < 1 {
		setting.MinimumSamples = 1
	} else if setting.MinimumSamples > 1000 {
		setting.MinimumSamples = 1000
	}
	if setting.UnavailableErrorRatePercent < 1 {
		setting.UnavailableErrorRatePercent = 1
	}
	if setting.UnavailableErrorRatePercent > 100 {
		setting.UnavailableErrorRatePercent = 100
	}
	if setting.StaleAfterMinutes < 1 {
		setting.StaleAfterMinutes = 1
	} else if setting.StaleAfterMinutes > 7*24*60 {
		setting.StaleAfterMinutes = 7 * 24 * 60
	}
	if setting.RetentionDays < 1 {
		setting.RetentionDays = 1
	} else if setting.RetentionDays > 365 {
		setting.RetentionDays = 365
	}
	return setting
}
