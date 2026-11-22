package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"strconv"
)

func ListChannelCostVersions(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("channel_id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"success": false, "message": "invalid channel_id"})
		return
	}
	var versions []model.ChannelCostVersion
	if err = model.DB.Where("channel_id = ?", id).Order("effective_at DESC, id DESC").Limit(200).Find(&versions).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": versions})
}

func CreateChannelCostVersion(c *gin.Context) {
	var input struct {
		ChannelId   int    `json:"channel_id"`
		Multiplier  string `json:"multiplier"`
		EffectiveAt int64  `json:"effective_at"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid cost version"})
		return
	}
	if input.EffectiveAt == 0 {
		input.EffectiveAt = common.GetTimestamp()
	}
	v, err := model.SaveChannelCostVersion(input.ChannelId, c.GetInt("id"), input.Multiplier, input.EffectiveAt)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.NotifyBusinessEventDelivery()
	c.JSON(200, gin.H{"success": true, "data": v})
}
