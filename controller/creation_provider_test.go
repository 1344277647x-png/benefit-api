package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreationProviderModelsRespectGroupsAndChannelState(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemory, oldRedis := common.MemoryCacheEnabled, common.RedisEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldGroups := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	oldPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = oldMemory, oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		model.InvalidatePricingCache()
	})
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private-image":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"无限制-Seedream-5-Pro":0.01,"无限制-NovelAI-V5":0.01,"gpt-image-2":0.01}`))
	for _, channel := range []model.Channel{
		{Id: 701, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "test-enabled"},
		{Id: 702, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Name: "test-disabled"},
	} {
		require.NoError(t, db.Create(&channel).Error)
	}
	for _, ability := range []model.Ability{
		{ChannelId: 701, Group: "default", Model: "无限制-Seedream-5-Pro", Enabled: true},
		{ChannelId: 701, Group: "default", Model: "无限制-NovelAI-V5", Enabled: true},
		{ChannelId: 701, Group: "default", Model: "gpt-image-2", Enabled: true},
		{ChannelId: 701, Group: "private-image", Model: "无限制-NovelAI-V4.5", Enabled: true},
		{ChannelId: 702, Group: "default", Model: "无限制-Seedream-5-Pro-2K", Enabled: true},
	} {
		require.NoError(t, db.Create(&ability).Error)
	}
	model.InvalidatePricingCache()
	user := &model.UserBase{Group: "default"}
	models, err := creationModelsForUser(user)
	require.NoError(t, err)
	require.Len(t, models, 3)
	byName := make(map[string]creationModel)
	for _, item := range models {
		byName[item.ID] = item
	}
	seedream := byName["无限制-Seedream-5-Pro"]
	assert.Equal(t, "seedream-image", seedream.Protocol)
	assert.Equal(t, []string{"default"}, seedream.Groups)
	assert.Equal(t, 1, seedream.Capabilities.MaxCount)
	assert.False(t, seedream.Capabilities.ReferenceImage)
	assert.Equal(t, []string{"2048x2048", "2304x1728", "1728x2304"}, seedream.Capabilities.Sizes)
	novel := byName["无限制-NovelAI-V5"]
	assert.Equal(t, "novelai-image", novel.Protocol)
	assert.Equal(t, 1, novel.Capabilities.MaxCount)
	assert.Empty(t, novel.Capabilities.Qualities)
	assert.Equal(t, []string{"1024x1024", "1216x832", "832x1216"}, novel.Capabilities.Sizes)
	assert.Equal(t, "openai-image", byName["gpt-image-2"].Protocol)
	assert.True(t, byName["gpt-image-2"].Capabilities.ReferenceImage)
	assert.True(t, creationModelAvailable(user, novel.ID, "image", novel.Protocol, "default"))
	assert.False(t, creationModelAvailable(user, novel.ID, "image", novel.Protocol, "private-image"))
	assert.False(t, creationModelAvailable(user, novel.ID, "image", "seedream-image", "default"))
}
