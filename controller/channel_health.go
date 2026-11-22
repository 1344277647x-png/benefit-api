package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/channel_health_setting"

	"github.com/gin-gonic/gin"
)

func GetChannelHealth(c *gin.Context) {
	setting := channel_health_setting.GetSetting()
	views, err := service.GetChannelHealthViews(time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":                    views,
		"enabled":                  setting.Enabled,
		"window_minutes":           setting.WindowMinutes,
		"refresh_interval_seconds": setting.RefreshIntervalSeconds,
		"refreshed_at":             time.Now().Unix(),
	})
}

func GetChannelHealthHistory(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours < 1 {
		hours = 1
	}
	maxHours := channel_health_setting.GetSetting().RetentionDays * 24
	if hours > maxHours {
		hours = maxHours
	}
	items, err := model.GetChannelHealthHistory(channelID, time.Now().Add(-time.Duration(hours)*time.Hour).Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items})
}

func GetPublicChannelHealth(c *gin.Context) {
	setting := channel_health_setting.GetSetting()
	items, err := service.GetPublicModelHealth(time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":                    items,
		"enabled":                  setting.Enabled,
		"refresh_interval_seconds": setting.RefreshIntervalSeconds,
		"refreshed_at":             time.Now().Unix(),
	})
}
