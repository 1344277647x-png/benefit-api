package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGetUserUsableGroupsAppliesRulesOnlyToMatchingUserGroup(t *testing.T) {
	originalGroups := setting.UserUsableGroups2JSONString()
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecialGroups := specialGroups.ReadAll()

	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default":"默认分组",
		"vip":"VIP 分组"
	}`))
	specialGroups.Clear()
	specialGroups.Set("vip", map[string]string{
		"+:premium": "Premium 分组",
	})

	regularUserGroups := GetUserUsableGroups("default")
	privilegedUserGroups := GetUserUsableGroups("vip")

	require.NotContains(t, regularUserGroups, "premium")
	require.Contains(t, privilegedUserGroups, "premium")
}

func TestGetUserUsableGroupsAppliesRemovalOnlyToMatchingUserGroup(t *testing.T) {
	originalGroups := setting.UserUsableGroups2JSONString()
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	originalSpecialGroups := specialGroups.ReadAll()

	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
		specialGroups.Clear()
		specialGroups.AddAll(originalSpecialGroups)
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default":"默认分组",
		"vip":"VIP 分组"
	}`))
	specialGroups.Clear()
	specialGroups.Set("vip", map[string]string{
		"-:default": "",
	})

	regularUserGroups := GetUserUsableGroups("default")
	privilegedUserGroups := GetUserUsableGroups("vip")

	require.Contains(t, regularUserGroups, "default")
	require.NotContains(t, privilegedUserGroups, "default")
}
