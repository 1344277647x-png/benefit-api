package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetReferralOverview(c *gin.Context) {
	overview, err := model.GetReferralOverview(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"enabled":            overview.Enabled,
		"compliance_ready":   overview.ComplianceReady,
		"affiliate_code":     overview.AffiliateCode,
		"invite_count":       overview.InviteCount,
		"qualified_count":    overview.QualifiedCount,
		"pending_quota":      overview.PendingQuota,
		"available_quota":    overview.AvailableQuota,
		"total_reward_quota": overview.TotalRewardQuota,
		"rules":              overview.Rules,
	})
}

func GetReferralRewards(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.GetReferralRewards(c.GetInt("id"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func GetReferralInvitees(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.GetReferralInvitees(c.GetInt("id"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
