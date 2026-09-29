package controller

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RequireTeamsEnabled(c *gin.Context) {
	if !operation_setting.TeamReleaseReady || !operation_setting.IsTeamEnabled() || !model.TeamTaskLogDeliverySupported() {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "团队订阅尚未开放"})
		return
	}
	c.Next()
}

func TeamSelf(c *gin.Context) {
	userId := c.GetInt("id")
	var invitations []model.TeamInvitation
	if err := model.DB.Where("target_user_id = ? AND status = ? AND expires_at > ?", userId, model.TeamInvitationPending, common.GetTimestamp()).
		Order("created_at desc").Find(&invitations).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	team, member, err := model.GetMyTeam(userId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiSuccess(c, gin.H{"team": nil, "invitations": invitations})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sub, period, err := model.TeamSubscriptionSummary(team.Id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		sub, period, err = nil, nil, nil
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var queued []model.TeamSubscription
	if err := model.DB.Where("team_id = ? AND status = ? AND start_time > ?", team.Id, model.TeamStatusActive, common.GetTimestamp()).
		Order("start_time asc").Find(&queued).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := gin.H{"team": team, "membership": member, "subscription": sub, "period": period, "queued": queued, "invitations": invitations}
	if member.Role == "owner" {
		payment, err := model.GetPendingTeamPayment(userId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if payment != nil {
			result["pending_payment"] = gin.H{
				"plan_title": payment.PlanTitle, "money": payment.Money,
				"payment_method": payment.PaymentMethod, "create_time": payment.CreateTime,
			}
		}
		var members []model.TeamMember
		var sent []model.TeamInvitation
		if err := model.DB.Where("team_id = ?", team.Id).Find(&members).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if err := model.DB.Where("team_id = ? AND status = ? AND expires_at > ?", team.Id, model.TeamInvitationPending, common.GetTimestamp()).Find(&sent).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		usage, err := model.TeamMemberUsage(team.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		result["members"], result["sent_invitations"], result["usage"] = members, sent, usage
		if period != nil {
			credited, err := model.TeamPeriodCreditedQuota(period.Id)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			result["period_credited_quota"] = credited
		}
	}
	common.ApiSuccess(c, result)
}

func TeamCreate(c *gin.Context) {
	var request struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "团队名称无效")
		return
	}
	team, err := model.CreateTeam(c.GetInt("id"), request.Name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, team)
}

func TeamPlans(c *gin.Context) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiSuccess(c, []model.SubscriptionPlan{})
		return
	}
	var plans []model.SubscriptionPlan
	if err := model.DB.Where("enabled = ? AND scope = ?", true, "team").Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plans)
}

func TeamMyUsage(c *gin.Context) {
	team, _, err := model.GetMyTeam(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	amount, requests, err := model.TeamOwnUsage(team.Id, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"amount": amount, "requests": requests})
}

// The request ID is returned by the relay in X-Oneapi-Request-Id. A settled
// charge does not prove the caller received its HTTP body; no result replay is
// promised until durable response delivery has been implemented.
func TeamOwnRequestStatus(c *gin.Context) {
	status, err := model.GetTeamMemberRequestStatus(c.Param("request_id"), c.GetInt("id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func TeamRecentRequestStatuses(c *gin.Context) {
	statuses, err := model.ListTeamMemberRequestStatuses(c.GetInt("id"), 50, c.Query("before"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, statuses)
}

func TeamInvite(c *gin.Context) {
	var request struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "邮箱无效")
		return
	}
	invitation, err := model.InviteTeamMember(c.GetInt("id"), request.Email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// The invitation is durable before notification. SMTP failure cannot revoke it.
	gopool.Go(func() {
		content := fmt.Sprintf("<p>您收到了 Benefit API 团队邀请，请登录网站的团队页面确认。</p><p>邀请编号：%d</p>", invitation.Id)
		if err := common.SendEmail("Benefit API 团队邀请", html.EscapeString(invitation.Email), content); err != nil {
			common.SysLog("team invitation notification failed")
		}
	})
	common.ApiSuccess(c, invitation)
}

func TeamRespondInvite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Accept bool `json:"accept"`
	}
	if err != nil || id <= 0 || c.ShouldBindJSON(&request) != nil {
		common.ApiErrorMsg(c, "邀请参数无效")
		return
	}
	if err := model.RespondToTeamInvitation(c.GetInt("id"), id, request.Accept); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TeamCancelInvite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "邀请编号无效")
		return
	}
	if err := model.CancelTeamInvitation(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TeamRemoveMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "成员编号无效")
		return
	}
	if err := model.RemoveTeamMember(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TeamListTokens(c *gin.Context) {
	team, _, err := model.GetMyTeam(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokens, err := model.ListTeamTokens(c.GetInt("id"), team.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summaries := make([]gin.H, 0, len(tokens))
	for _, token := range tokens {
		summaries = append(summaries, gin.H{"id": token.Id, "name": token.Name,
			"enabled": token.TeamEnabled, "created_time": token.CreatedTime})
	}
	common.ApiSuccess(c, summaries)
}

func TeamCreateToken(c *gin.Context) {
	var request struct {
		Name        string `json:"name"`
		Group       string `json:"group"`
		ModelLimits string `json:"model_limits"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "令牌参数无效")
		return
	}
	request.Group = strings.TrimSpace(request.Group)
	if request.Group != "" {
		if _, ok := service.GetUserUsableGroups(c.GetString("group"))[request.Group]; !ok || !ratio_setting.ContainsGroupRatio(request.Group) {
			common.ApiErrorMsg(c, "该账号无权使用此分组")
			return
		}
	}
	token, err := model.CreateTeamToken(c.GetInt("id"), request.Name, request.Group, request.ModelLimits)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": token.Id, "name": token.Name, "key": "sk-" + token.Key})
}

func TeamDisableToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "令牌编号无效")
		return
	}
	team, _, err := model.GetMyTeam(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DisableTeamToken(c.GetInt("id"), team.Id, id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TeamBalancePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var request struct {
		PlanId int `json:"plan_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.PlanId <= 0 {
		common.ApiErrorMsg(c, "套餐编号无效")
		return
	}
	if err := model.PurchaseTeamWithBalance(c.GetInt("id"), request.PlanId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminTeamList(c *gin.Context) {
	var teams []model.Team
	if err := model.DB.Order("id desc").Limit(100).Find(&teams).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, teams)
}

func AdminPendingTeamRequests(c *gin.Context) {
	audits, err := model.PendingTeamRequestAudits(100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, audits)
}

func AdminResolveTeamFailedAttempt(c *gin.Context) {
	var request struct {
		EvidenceRef string `json:"evidence_ref"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "核查凭据编号无效")
		return
	}
	applied, err := model.ResolveTeamAttemptAsFailed(c.Param("request_id"), c.GetInt("id"), request.EvidenceRef)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"applied": applied})
}

// Only provider-confirmed charges can be reconciled here. A recovered charge
// remains marked withheld; the original response cannot be replayed safely.
func AdminResolveTeamBilledAttempt(c *gin.Context) {
	var request struct {
		EvidenceRef string `json:"evidence_ref"`
		FinalQuota  int64  `json:"final_quota"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "核查凭据或实际费用无效")
		return
	}
	applied, err := model.ResolveTeamAttemptAsBilled(c.Param("request_id"), c.GetInt("id"), request.EvidenceRef, request.FinalQuota)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var usage model.TeamUsage
	if err := model.DB.Select("status", "pending_final").Where("request_id = ?", c.Param("request_id")).First(&usage).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	status := "settled_withheld"
	if usage.PendingFinal {
		status = "pending_settlement"
	}
	common.ApiSuccess(c, gin.H{"applied": applied, "status": status})
}

// This route records an explicit platform loss after the provider billed an
// undelivered result that cannot fit in the request's original quota period.
func AdminCompensateTeamUndeliveredCharge(c *gin.Context) {
	var request struct {
		EvidenceRef string `json:"evidence_ref"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "核查凭据编号无效")
		return
	}
	applied, unbilledQuota, err := model.CompensateTeamUndeliveredCharge(c.Param("request_id"), c.GetInt("id"), request.EvidenceRef)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"applied": applied, "status": "compensated_no_delivery", "unbilled_quota": unbilledQuota})
}

// An explicit goodwill credit for a settled, disputed response. It never
// erases actual provider usage or creates a second consumption log.
func AdminCreditTeamDeliveryDispute(c *gin.Context) {
	var request struct {
		EvidenceRef string `json:"evidence_ref"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "核查凭据编号无效")
		return
	}
	applied, creditedQuota, err := model.CreditTeamDeliveryDispute(c.Param("request_id"), c.GetInt("id"), request.EvidenceRef)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"applied": applied, "status": "delivery_dispute_credited", "credited_quota": creditedQuota})
}

func AdminTeamSuspend(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "团队编号无效")
		return
	}
	result := model.DB.Model(&model.Team{}).Where("id = ? AND status = ?", id, model.TeamStatusActive).
		Update("status", model.TeamStatusSuspended)
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		common.ApiErrorMsg(c, "团队不存在或已停用")
		return
	}
	common.ApiSuccess(c, nil)
}
