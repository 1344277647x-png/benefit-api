package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// Team billing can be stopped immediately through the operator setting.
type TeamSetting struct {
	Enabled bool `json:"enabled"`
}

// The release gate covers both purchases and relay credentials so they
// cannot be deployed independently. The operator setting remains the
// emergency stop for the full feature.
const TeamReleaseReady = true

var teamSetting = TeamSetting{Enabled: true}

func init() {
	config.GlobalConfig.Register("team_setting", &teamSetting)
}

func IsTeamEnabled() bool {
	return teamSetting.Enabled
}
