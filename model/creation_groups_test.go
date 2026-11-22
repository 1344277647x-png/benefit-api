package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestEnabledModelEndpointGroupsExcludeDisabledChannels(t *testing.T) {
	resetPricingEndpointTestTables(t)

	modelName := "gpt-image-1"
	require.NoError(t, DB.Create(&Channel{
		Id:     301,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "enabled-key",
		Status: common.ChannelStatusEnabled,
		Name:   "enabled-image",
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "image-enabled",
		Model:     modelName,
		ChannelId: 301,
		Enabled:   true,
	}).Error)

	require.NoError(t, DB.Create(&Channel{
		Id:     302,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "disabled-key",
		Status: common.ChannelStatusManuallyDisabled,
		Name:   "disabled-image",
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "image-disabled",
		Model:     modelName,
		ChannelId: 302,
		Enabled:   true,
	}).Error)

	groups, err := GetEnabledModelEndpointGroups()
	require.NoError(t, err)
	require.Equal(
		t,
		[]string{"image-enabled"},
		groups[modelName][constant.EndpointTypeImageGeneration],
	)
}
