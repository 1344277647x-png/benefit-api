package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	MaxReferralRewardRateBasisPoints = 10000
	MaxReferralSettlementDelayHours  = 24 * 30
)

type ReferralSetting struct {
	Enabled               bool `json:"enabled"`
	MinimumTopupQuota     int  `json:"minimum_topup_quota"`
	RewardRateBasisPoints int  `json:"reward_rate_basis_points"`
	InviteeBonusQuota     int  `json:"invitee_bonus_quota"`
	PerInviteeCapQuota    int  `json:"per_invitee_cap_quota"`
	MonthlyCapQuota       int  `json:"monthly_cap_quota"`
	SettlementDelayHours  int  `json:"settlement_delay_hours"`
}

var referralSetting = ReferralSetting{
	Enabled:               false,
	MinimumTopupQuota:     int(10 * common.QuotaPerUnit),
	RewardRateBasisPoints: 300,
	InviteeBonusQuota:     int(0.5 * common.QuotaPerUnit),
	PerInviteeCapQuota:    int(5 * common.QuotaPerUnit),
	MonthlyCapQuota:       int(50 * common.QuotaPerUnit),
	SettlementDelayHours:  72,
}

func init() {
	config.GlobalConfig.Register("referral_setting", &referralSetting)
}

func GetReferralSetting() *ReferralSetting {
	return &referralSetting
}

func GetReferralSettingSnapshot() ReferralSetting {
	setting := referralSetting
	setting.MinimumTopupQuota = max(setting.MinimumTopupQuota, 0)
	setting.RewardRateBasisPoints = min(max(setting.RewardRateBasisPoints, 0), MaxReferralRewardRateBasisPoints)
	setting.InviteeBonusQuota = max(setting.InviteeBonusQuota, 0)
	setting.PerInviteeCapQuota = max(setting.PerInviteeCapQuota, 0)
	setting.MonthlyCapQuota = max(setting.MonthlyCapQuota, 0)
	setting.SettlementDelayHours = min(max(setting.SettlementDelayHours, 0), MaxReferralSettlementDelayHours)
	return setting
}

func IsReferralEnabled() bool {
	return referralSetting.Enabled
}
