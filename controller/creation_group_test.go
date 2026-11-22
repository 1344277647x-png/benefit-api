package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestCompatibleCreationGroupsIntersectsAndSortsAccountFirst(t *testing.T) {
	groups := compatibleCreationGroups(
		[]string{"vip", "default", "inactive", "vip"},
		[]string{"all"},
		map[string]string{
			"default":  "Default",
			"inactive": "Inactive",
			"vip":      "VIP",
		},
		"default",
	)

	require.Equal(t, []string{"default", "vip"}, groups)
}

func TestCompatibleCreationGroupsHonorsPricingGroups(t *testing.T) {
	previous := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previous)) })
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))

	groups := compatibleCreationGroups(
		[]string{"default", "vip", "svip"},
		[]string{"svip"},
		map[string]string{
			"default": "Default",
			"vip":     "VIP",
			"svip":    "SVIP",
		},
		"default",
	)

	require.Equal(t, []string{"svip"}, groups)
}
