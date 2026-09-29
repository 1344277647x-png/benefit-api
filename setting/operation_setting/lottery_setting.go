package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const LotteryDrawThresholdCents int64 = 5000

type LotterySetting struct {
	Enabled bool  `json:"enabled"`
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

var lotterySetting = LotterySetting{
	Enabled: false,
	StartAt: 0,
	EndAt:   0,
}

func init() {
	config.GlobalConfig.Register("lottery_setting", &lotterySetting)
}

func GetLotterySetting() *LotterySetting {
	return &lotterySetting
}

func GetLotterySettingSnapshot() LotterySetting {
	setting := lotterySetting
	if setting.StartAt < 0 {
		setting.StartAt = 0
	}
	if setting.EndAt < 0 {
		setting.EndAt = 0
	}
	return setting
}

func IsLotteryActiveAt(timestamp int64) bool {
	setting := GetLotterySettingSnapshot()
	if !setting.Enabled {
		return false
	}
	if setting.StartAt > 0 && timestamp < setting.StartAt {
		return false
	}
	return setting.EndAt == 0 || timestamp <= setting.EndAt
}
