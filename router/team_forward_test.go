package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamPastRequestAuditAvailableWhenTeamFeatureClosed(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}))
	user := &model.User{Username: "former-team-audit", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "audit-local-only"}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(&model.TeamUsage{RequestId: "past-team-request", TeamId: 99,
		MemberUserId: user.Id, Reserved: 30, FinalAmount: 30, Status: "settled"}).Error)
	require.NoError(t, model.DB.Create(&model.TeamSyncBillingEvent{RequestId: "past-team-request", TeamId: 99,
		UserId: user.Id, FinalQuota: 30, Ready: true, ResponseState: "write_failed"}).Error)
	other := &model.User{Username: "other-former-team", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "other-audit-local"}
	require.NoError(t, model.DB.Create(other).Error)
	require.NoError(t, model.DB.Create(&model.TeamUsage{RequestId: "private-other-request", TeamId: 100,
		MemberUserId: other.Id, Reserved: 20, Status: "reserved"}).Error)
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "local-team-audit")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)
	request := httptest.NewRequest(http.MethodGet, "/api/team/requests/past-team-request", nil)
	request.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"response_state":"write_failed"`)
	assert.NotContains(t, response.Body.String(), "team_id")
	request = httptest.NewRequest(http.MethodGet, "/api/team/requests", nil)
	request.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"request_id":"past-team-request"`)
	assert.Contains(t, response.Body.String(), `"response_state":"write_failed"`)
	assert.NotContains(t, response.Body.String(), "private-other-request")
	assert.NotContains(t, response.Body.String(), "team_id")
	request = httptest.NewRequest(http.MethodGet, "/api/team/requests/past-team-request", nil)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestTeamProviderConfirmedBillingReconciliationRequiresAdminSession(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.Team{}, &model.TeamMember{},
		&model.TeamSubscription{}, &model.TeamQuotaPeriod{}, &model.TeamUsage{},
		&model.TeamSyncBillingEvent{}, &model.TeamSyncLogReceipt{}, &model.Log{}))
	admin := &model.User{Username: "team-reviewer", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "team-reviewer-local"}
	member := &model.User{Username: "team-reconcile-member", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "team-reconcile-local"}
	require.NoError(t, model.DB.Create(admin).Error)
	require.NoError(t, model.DB.Create(member).Error)
	team, err := model.CreateTeam(member.Id, "Audited team")
	require.NoError(t, err)
	now := time.Now().Unix()
	sub := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: model.SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}
	require.NoError(t, model.DB.Create(sub).Error)
	period := &model.TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: now - 60, EndTime: now + 3600,
		AmountTotal: 100, AmountUsed: 30}
	require.NoError(t, model.DB.Create(period).Error)
	usage := &model.TeamUsage{RequestId: "local-billed-audit", TeamId: team.Id, MemberUserId: member.Id,
		TeamSubscriptionId: sub.Id, PeriodId: period.Id, Reserved: 30, AttemptedAt: now,
		Status: "reserved", CreatedAt: now}
	require.NoError(t, model.DB.Create(usage).Error)
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "local-admin-review")
	require.NoError(t, err)
	memberSession, err := service.CreateLoginSession(member.Id, "password", "127.0.0.1", "local-member-review")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)
	send := func(auth, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/team/admin/pending-requests/local-billed-audit/confirm-billed", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if auth != "" {
			request.Header.Set("Authorization", "Bearer "+auth)
		}
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	body := `{"evidence_ref":"provider-audit:12345","final_quota":50}`
	assert.Equal(t, http.StatusUnauthorized, send("", body).Code)
	assert.Equal(t, http.StatusForbidden, send(memberSession.AccessToken, body).Code)
	first := send(adminSession.AccessToken, body)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"status":"settled_withheld"`)
	assert.Contains(t, first.Body.String(), `"applied":true`)
	second := send(adminSession.AccessToken, body)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"applied":false`)
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/team/requests/local-billed-audit", nil)
	statusRequest.Header.Set("Authorization", "Bearer "+memberSession.AccessToken)
	statusResponse := httptest.NewRecorder()
	engine.ServeHTTP(statusResponse, statusRequest)
	require.Equal(t, http.StatusOK, statusResponse.Code)
	assert.Contains(t, statusResponse.Body.String(), `"response_state":"withheld"`)
	assert.NotContains(t, statusResponse.Body.String(), "provider-audit")
}

func TestTeamCompensationRouteRequiresAdminAndPreservesMemberPrivacy(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.Team{}, &model.TeamMember{},
		&model.TeamSubscription{}, &model.TeamQuotaPeriod{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}))
	admin := &model.User{Username: "compensation-reviewer", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "compensation-reviewer-local"}
	member := &model.User{Username: "compensation-member", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "compensation-member-local"}
	require.NoError(t, model.DB.Create(admin).Error)
	require.NoError(t, model.DB.Create(member).Error)
	team, err := model.CreateTeam(member.Id, "Compensated team")
	require.NoError(t, err)
	now := time.Now().Unix()
	sub := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: model.SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}
	require.NoError(t, model.DB.Create(sub).Error)
	period := &model.TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: now - 60, EndTime: now + 3600,
		AmountTotal: 100, AmountUsed: 100}
	require.NoError(t, model.DB.Create(period).Error)
	usage := &model.TeamUsage{RequestId: "local-compensation", TeamId: team.Id, MemberUserId: member.Id,
		TeamSubscriptionId: sub.Id, PeriodId: period.Id, Reserved: 70, PendingFinal: true,
		PendingFinalAmount: 90, AttemptedAt: now, Status: "reserved", CreatedAt: now}
	require.NoError(t, model.DB.Create(usage).Error)
	require.NoError(t, model.DB.Create(&model.TeamSyncBillingEvent{RequestId: usage.RequestId,
		TeamId: team.Id, UserId: member.Id, FinalQuota: 90, ResponseState: "withheld"}).Error)
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "local-compensation-admin")
	require.NoError(t, err)
	memberSession, err := service.CreateLoginSession(member.Id, "password", "127.0.0.1", "local-compensation-member")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)
	send := func(token, evidence string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost,
			"/api/team/admin/pending-requests/local-compensation/compensate",
			strings.NewReader(`{"evidence_ref":"`+evidence+`"}`))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	assert.Equal(t, http.StatusUnauthorized, send("", "provider:case-123").Code)
	assert.Equal(t, http.StatusForbidden, send(memberSession.AccessToken, "provider:case-123").Code)
	first := send(adminSession.AccessToken, "provider:case-123")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"applied":true`)
	assert.Contains(t, first.Body.String(), `"unbilled_quota":90`)
	second := send(adminSession.AccessToken, "provider:case-123")
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"applied":false`)
	request := httptest.NewRequest(http.MethodGet, "/api/team/requests/local-compensation", nil)
	request.Header.Set("Authorization", "Bearer "+memberSession.AccessToken)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"response_state":"compensated_no_delivery"`)
	assert.Contains(t, response.Body.String(), `"unbilled_quota":90`)
	assert.NotContains(t, response.Body.String(), "provider:case-123")
	assert.NotContains(t, response.Body.String(), "reviewer_id")
}

func TestSettledTeamDeliveryCreditRequiresAdminAndPreservesGrossLog(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.Team{}, &model.TeamMember{},
		&model.TeamSubscription{}, &model.TeamQuotaPeriod{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}))
	admin := &model.User{Username: "delivery-reviewer", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "delivery-reviewer-local"}
	member := &model.User{Username: "delivery-member", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "delivery-member-local"}
	require.NoError(t, model.DB.Create(admin).Error)
	require.NoError(t, model.DB.Create(member).Error)
	team, err := model.CreateTeam(member.Id, "Delivery audit team")
	require.NoError(t, err)
	now := time.Now().Unix()
	sub := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
		ResetPeriod: model.SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}
	require.NoError(t, model.DB.Create(sub).Error)
	period := &model.TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: now - 60,
		EndTime: now + 3600, AmountTotal: 100, AmountUsed: 30}
	require.NoError(t, model.DB.Create(period).Error)
	usage := &model.TeamUsage{RequestId: "local-delivery-credit", TeamId: team.Id, MemberUserId: member.Id,
		TeamSubscriptionId: sub.Id, PeriodId: period.Id, Reserved: 30, FinalAmount: 30,
		Status: "settled", AttemptedAt: now, CreatedAt: now}
	require.NoError(t, model.DB.Create(usage).Error)
	require.NoError(t, model.DB.Create(&model.TeamSyncBillingEvent{RequestId: usage.RequestId,
		TeamId: team.Id, UserId: member.Id, FinalQuota: 30, Ready: true, ResponseState: "withheld"}).Error)
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "delivery-review")
	require.NoError(t, err)
	memberSession, err := service.CreateLoginSession(member.Id, "password", "127.0.0.1", "delivery-member")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)
	send := func(token, evidence string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost,
			"/api/team/admin/requests/local-delivery-credit/credit-delivery-dispute",
			strings.NewReader(`{"evidence_ref":"`+evidence+`"}`))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	assert.Equal(t, http.StatusUnauthorized, send("", "case:one-123").Code)
	assert.Equal(t, http.StatusForbidden, send(memberSession.AccessToken, "case:one-123").Code)
	first := send(adminSession.AccessToken, "case:one-123")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"credited_quota":30`)
	assert.Contains(t, first.Body.String(), `"applied":true`)
	second := send(adminSession.AccessToken, "case:one-123")
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"applied":false`)
	request := httptest.NewRequest(http.MethodGet, "/api/team/requests/local-delivery-credit", nil)
	request.Header.Set("Authorization", "Bearer "+memberSession.AccessToken)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"credited_quota":30`)
	assert.NotContains(t, response.Body.String(), "case:one-123")
	assert.NotContains(t, response.Body.String(), "credit_by")
}

func TestTeamSignedEpayCallbackThroughPublicRouterCreditsExactlyOnce(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Team{}, &model.TeamMember{}, &model.TeamInvitation{}, &model.TeamSubscription{},
		&model.TeamOrder{}, &model.UserSubscription{}, &model.TopUp{}, &model.BusinessEvent{}))
	previousAddress, previousId, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://local-payment.example.test"
	operation_setting.EpayId = "local-team-merchant"
	operation_setting.EpayKey = "local-signed-callback-only"
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousId, previousKey
	})
	owner := &model.User{Username: "epay-team-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", Quota: 1000, AffCode: "epay-team-owner"}
	require.NoError(t, model.DB.Create(owner).Error)
	team, err := model.CreateTeam(owner.Id, "Local callback team")
	require.NoError(t, err)
	order := &model.TeamOrder{TeamId: team.Id, PayerUserId: owner.Id, PlanId: 3, PlanTitle: "Local shared plan",
		SeatLimit: 2, AmountTotal: 1000, DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1,
		ResetPeriod: model.SubscriptionResetNever, Money: 1.25, TradeNo: "local-epay-team-order",
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, model.DB.Create(order).Error)
	form := url.Values{}
	for key, value := range epay.GenerateParams(map[string]string{
		"pid": operation_setting.EpayId, "out_trade_no": order.TradeNo,
		"trade_no": "local-epay-transaction", "money": "1.25", "type": "alipay",
		"trade_status": epay.StatusTradeSuccess, "sign_type": "MD5",
	}, operation_setting.EpayKey) {
		form.Set(key, value)
	}
	engine := gin.New()
	SetApiRouter(engine)
	callback := func(values url.Values) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/team/epay/notify", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	tampered := url.Values{}
	for key, values := range form {
		tampered[key] = append([]string(nil), values...)
	}
	tampered.Set("money", "2.25")
	assert.Equal(t, "fail", callback(tampered).Body.String())
	returnRecorder := httptest.NewRecorder()
	returnRequest := httptest.NewRequest(http.MethodGet, "/api/team/epay/return?"+form.Encode(), nil)
	engine.ServeHTTP(returnRecorder, returnRequest)
	assert.Equal(t, http.StatusFound, returnRecorder.Code)
	assert.Contains(t, returnRecorder.Header().Get("Location"), "pay=pending")
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	for i := 0; i < 2; i++ {
		response := callback(form)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "success", response.Body.String())
	}
	var subscriptions []model.TeamSubscription
	require.NoError(t, model.DB.Where("team_id = ?", team.Id).Find(&subscriptions).Error)
	require.Len(t, subscriptions, 1)
	assert.EqualValues(t, 1000, subscriptions[0].AmountTotal)
	require.NoError(t, model.DB.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, owner.Id).Error)
	assert.Equal(t, owner.Quota, stored.Quota, "a team epay order must not mutate the personal wallet")
	var personalTerms, topups int64
	require.NoError(t, model.DB.Model(&model.UserSubscription{}).Count(&personalTerms).Error)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&topups).Error)
	assert.Zero(t, personalTerms)
	assert.Zero(t, topups)
}

// Production purchase/token routes stay closed. This isolated handler harness
// verifies the real authenticated HTTP contract and database effects after
// the gate, without a production setting or a payment provider.
func TestTeamAuthenticatedPurchaseInviteAndRevocationAPI(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.SubscriptionPlan{},
		&model.Team{}, &model.TeamMember{}, &model.TeamInvitation{}, &model.TeamSubscription{},
		&model.TeamQuotaPeriod{}, &model.TeamOrder{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.BusinessEvent{}))
	settings := operation_setting.GetPaymentSetting()
	previousPayment := *settings
	settings.ComplianceConfirmed = true
	settings.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() { *settings = previousPayment })
	owner := &model.User{Username: "local-team-api-owner", Email: "team-owner@example.test",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		Quota: 2_000_000, AffCode: "local-team-api-owner"}
	member := &model.User{Username: "local-team-api-member", Email: "team-member@example.test",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		Quota: 1000, AffCode: "local-team-api-member"}
	third := &model.User{Username: "local-team-api-third", Email: "team-third@example.test",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
		Quota: 1000, AffCode: "local-team-api-third"}
	require.NoError(t, model.DB.Create(owner).Error)
	require.NoError(t, model.DB.Create(member).Error)
	require.NoError(t, model.DB.Create(third).Error)
	plan := &model.SubscriptionPlan{Scope: "team", SeatLimit: 2, Title: "Local shared month",
		Enabled: true, PriceAmount: 1, TotalAmount: 1000,
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	ownerSession, err := service.CreateLoginSession(owner.Id, "password", "127.0.0.1", "local-team-owner")
	require.NoError(t, err)
	memberSession, err := service.CreateLoginSession(member.Id, "password", "127.0.0.1", "local-team-member")
	require.NoError(t, err)
	engine := gin.New()
	teamRoutes := engine.Group("/api/team", middleware.UserAuth())
	teamRoutes.POST("/", controller.TeamCreate)
	teamRoutes.GET("/self", controller.TeamSelf)
	teamRoutes.GET("/plans", controller.TeamPlans)
	teamRoutes.POST("/balance/pay", controller.TeamBalancePay)
	teamRoutes.POST("/invitations", controller.TeamInvite)
	teamRoutes.POST("/invitations/:id/respond", controller.TeamRespondInvite)
	teamRoutes.POST("/tokens", controller.TeamCreateToken)
	teamRoutes.DELETE("/tokens/:id", controller.TeamDisableToken)
	teamRoutes.DELETE("/members/:id", controller.TeamRemoveMember)
	send := func(method, path, bearer, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+bearer)
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	ownerAccess, memberAccess := ownerSession.AccessToken, memberSession.AccessToken
	created := send(http.MethodPost, "/api/team/", ownerAccess, `{"name":"Local QA team"}`)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	assert.Contains(t, created.Body.String(), `"success":true`)
	assert.Contains(t, send(http.MethodGet, "/api/team/plans", ownerAccess, "").Body.String(), "Local shared month")
	pay := fmt.Sprintf(`{"plan_id":%d}`, plan.Id)
	paid := send(http.MethodPost, "/api/team/balance/pay", ownerAccess, pay)
	require.Equal(t, http.StatusOK, paid.Code, paid.Body.String())
	assert.Contains(t, paid.Body.String(), `"success":true`)
	var team model.Team
	require.NoError(t, model.DB.Where("owner_id = ?", owner.Id).First(&team).Error)
	var firstTerm model.TeamSubscription
	require.NoError(t, model.DB.Where("team_id = ?", team.Id).First(&firstTerm).Error)
	assert.EqualValues(t, 1000, firstTerm.AmountTotal)
	invited := send(http.MethodPost, "/api/team/invitations", ownerAccess, `{"email":"team-member@example.test"}`)
	require.Equal(t, http.StatusOK, invited.Code, invited.Body.String())
	var invite model.TeamInvitation
	require.NoError(t, model.DB.Where("team_id = ? AND target_user_id = ?", team.Id, member.Id).First(&invite).Error)
	blocked := send(http.MethodPost, "/api/team/invitations", ownerAccess, `{"email":"team-third@example.test"}`)
	assert.NotContains(t, blocked.Body.String(), `"success":true`)
	var thirdInvites int64
	require.NoError(t, model.DB.Model(&model.TeamInvitation{}).Where("target_user_id = ?", third.Id).Count(&thirdInvites).Error)
	assert.Zero(t, thirdInvites)
	accepted := send(http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/respond", invite.Id), memberAccess, `{"accept":true}`)
	require.Equal(t, http.StatusOK, accepted.Code, accepted.Body.String())
	assert.Contains(t, accepted.Body.String(), `"success":true`)
	memberView := send(http.MethodGet, "/api/team/self", memberAccess, "")
	assert.Contains(t, memberView.Body.String(), `"role":"member"`)
	assert.NotContains(t, memberView.Body.String(), "sent_invitations")
	createdKey := send(http.MethodPost, "/api/team/tokens", memberAccess, `{"name":"Local member key"}`)
	require.Equal(t, http.StatusOK, createdKey.Code, createdKey.Body.String())
	assert.Contains(t, createdKey.Body.String(), `"success":true`)
	var token model.Token
	require.NoError(t, model.DB.Where("team_id = ? AND user_id = ?", team.Id, member.Id).First(&token).Error)
	assert.NotEmpty(t, token.Key)
	renewed := send(http.MethodPost, "/api/team/balance/pay", ownerAccess, pay)
	require.Equal(t, http.StatusOK, renewed.Code, renewed.Body.String())
	var terms []model.TeamSubscription
	require.NoError(t, model.DB.Where("team_id = ?", team.Id).Order("start_time asc").Find(&terms).Error)
	require.Len(t, terms, 2)
	assert.Equal(t, terms[0].EndTime, terms[1].StartTime, "early renewal must queue behind the active term")
	assert.EqualValues(t, 1000, terms[1].AmountTotal, "next term starts with fresh quota")
	require.NoError(t, model.DB.First(owner, owner.Id).Error)
	assert.Less(t, owner.Quota, 2_000_000, "balance purchase debits only owner wallet")
	require.NoError(t, model.DB.First(member, member.Id).Error)
	assert.Equal(t, 1000, member.Quota)
	left := send(http.MethodDelete, fmt.Sprintf("/api/team/members/%d", member.Id), memberAccess, "")
	require.Equal(t, http.StatusOK, left.Code, left.Body.String())
	_, err = model.ValidateTeamToken(token.Key)
	assert.Error(t, err, "leaving the team must revoke the credential immediately")
	assert.Contains(t, send(http.MethodGet, "/api/team/self", ownerAccess, "").Body.String(), "Local QA team")
}

// The production TokenAuth gate remains closed. This isolated route validates
// a real team credential and exercises the downstream relay and settlement;
// it does not stand in for a public-route acceptance test.
func TestTeamTokenForwardsProtocolsToLocalUpstreamAndSettlesSharedQuota(t *testing.T) {
	setupRelayRouterTestDB(t)
	previousLogConsume := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsume })
	previousPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices)) })
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o-mini":0.001}`))
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelHealthSnapshot{}, &model.ChannelHealthBucket{},
		&model.Team{}, &model.TeamMember{}, &model.TeamSubscription{},
		&model.TeamQuotaPeriod{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.TeamSyncLogReceipt{}, &model.Log{}))

	owner := &model.User{Username: "forward-team-owner", Status: common.UserStatusEnabled,
		Group: "default", Quota: 1000, AffCode: "forward-local-only"}
	require.NoError(t, model.DB.Create(owner).Error)
	team, err := model.CreateTeam(owner.Id, "Local forwarding test")
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TeamSubscription{TeamId: team.Id, SeatLimit: 2,
		AmountTotal: 1000000, ResetPeriod: model.SubscriptionResetNever,
		StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}).Error)
	token, err := model.CreateTeamToken(owner.Id, "Local relay", "default", "")
	require.NoError(t, err)
	var failChat atomic.Bool
	var failedChatAttempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		if failChat.Load() && r.URL.Path == "/v1/chat/completions" {
			failedChatAttempts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"local upstream failure"}}`))
			return
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"id":"chatcmpl-local","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
		case "/v1/responses":
			_, _ = w.Write([]byte(`{"id":"resp_local","object":"response","created_at":1,"model":"gpt-4o-mini","status":"completed","output":[{"id":"msg_local","type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/responses/compact":
			_, _ = w.Write([]byte(`{"id":"compact_local","object":"response.compaction","created_at":1,"model":"gpt-4o-mini","output":[{"type":"compaction","summary":"pong"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/embeddings":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"gpt-4o-mini","usage":{"prompt_tokens":2,"total_tokens":2}}`))
		case "/v1/images/generations":
			_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"aGVsbG8="}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/rerank":
			_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.8}],"usage":{"total_tokens":3}}`))
		case "/v1/alpha/search":
			_, _ = w.Write([]byte(`{"results":[{"title":"pong"}]}`))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(make([]byte, 48000)) // one second of 24 kHz mono 16-bit PCM
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "local-team-relay",
		Key: "local-test-upstream", Status: common.ChannelStatusEnabled,
		BaseURL: &upstream.URL, Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, model.DB.Create(channel).Error)
	alphaChannel := &model.Channel{Type: constant.ChannelTypeNewAPI, Name: "local-team-search",
		Key: "local-test-search", Status: common.ChannelStatusEnabled,
		BaseURL: &upstream.URL, Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, model.DB.Create(alphaChannel).Error)

	engine := gin.New()
	testCases := []struct {
		name, path, body, responseText string
		format                         types.RelayFormat
	}{
		{"chat", "/v1/chat/completions", `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`, "pong", types.RelayFormatOpenAI},
		{"claude", "/v1/messages", `{"model":"gpt-4o-mini","max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`, "pong", types.RelayFormatClaude},
		{"gemini", "/v1beta/models/gpt-4o-mini:generateContent", `{"contents":[{"role":"user","parts":[{"text":"ping"}]}],"generationConfig":{"maxOutputTokens":8}}`, "pong", types.RelayFormatGemini},
		{"responses", "/v1/responses", `{"model":"gpt-4o-mini","input":"ping","max_output_tokens":8}`, "pong", types.RelayFormatOpenAIResponses},
		{"responses-compact", "/v1/responses/compact", `{"model":"gpt-4o-mini","input":"ping"}`, "pong", types.RelayFormatOpenAIResponsesCompaction},
		{"embeddings", "/v1/embeddings", `{"model":"gpt-4o-mini","input":"ping"}`, "embedding", types.RelayFormatEmbedding},
		{"images", "/v1/images/generations", `{"model":"gpt-4o-mini","prompt":"ping","n":1}`, "aGVsbG8=", types.RelayFormatOpenAIImage},
		{"audio", "/v1/audio/speech", `{"model":"gpt-4o-mini","input":"ping","voice":"alloy","response_format":"pcm"}`, "", types.RelayFormatOpenAIAudio},
		{"rerank", "/v1/rerank", `{"model":"gpt-4o-mini","query":"ping","documents":["pong"]}`, "relevance_score", types.RelayFormatRerank},
		{"alpha-search", "/v1/alpha/search", `{"model":"gpt-4o-mini","query":"ping"}`, "pong", types.RelayFormatOpenAIAlphaSearch},
	}
	forward := func(c *gin.Context) {
		validated, err := model.ValidateTeamToken(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		user, err := model.GetUserCache(validated.UserId)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		user.WriteContext(c)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if err := middleware.SetupContextForToken(c, validated); err != nil {
			return
		}
		selected := channel
		if c.Request.URL.Path == "/v1/alpha/search" {
			selected = alphaChannel
		}
		if apiErr := middleware.SetupContextForSelectedChannel(c, selected, "gpt-4o-mini"); apiErr != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		for _, tc := range testCases {
			if tc.path == c.Request.URL.Path {
				controller.Relay(c, tc.format)
				return
			}
		}
		c.AbortWithStatus(http.StatusNotFound)
	}
	for _, tc := range testCases {
		engine.POST(tc.path, forward)
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token.Key)
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), tc.responseText)
			assert.NotEmpty(t, recorder.Body.Bytes())
		})
	}
	previousRetryTimes := common.RetryTimes
	common.RetryTimes = 2
	defer func() { common.RetryTimes = previousRetryTimes }()
	failChat.Store(true)
	failure := httptest.NewRecorder()
	failureRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`))
	failureRequest.Header.Set("Content-Type", "application/json")
	failureRequest.Header.Set("Authorization", "Bearer "+token.Key)
	engine.ServeHTTP(failure, failureRequest)
	failChat.Store(false)
	assert.NotEqual(t, http.StatusOK, failure.Code)
	assert.EqualValues(t, 1, failedChatAttempts.Load(), "an attempted team request cannot be sent upstream twice")
	streamRecorder := httptest.NewRecorder()
	streamRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"stream":true}`))
	streamRequest.Header.Set("Content-Type", "application/json")
	streamRequest.Header.Set("Authorization", "Bearer "+token.Key)
	engine.ServeHTTP(streamRecorder, streamRequest)
	assert.Equal(t, http.StatusServiceUnavailable, streamRecorder.Code)
	assert.NotContains(t, streamRecorder.Body.String(), "pong")
	var usages []model.TeamUsage
	require.NoError(t, model.DB.Where("team_id = ? AND member_user_id = ?", team.Id, owner.Id).Find(&usages).Error)
	require.Len(t, usages, len(testCases)+1)
	var totalQuota int64
	for _, usage := range usages {
		if usage.Status != "settled" {
			assert.Equal(t, "reserved", usage.Status)
			assert.NotZero(t, usage.AttemptedAt)
			continue
		}
		assert.Equal(t, "settled", usage.Status)
		assert.Positive(t, usage.FinalAmount)
		var event model.TeamSyncBillingEvent
		require.NoError(t, model.DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
		assert.Equal(t, "server_write_completed", event.ResponseState)
		totalQuota += usage.FinalAmount
	}
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", owner.Id, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, len(testCases))
	var loggedQuota int64
	for _, log := range logs {
		assert.Equal(t, "default", log.Group)
		assert.Equal(t, "Local relay", log.TokenName)
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
		assert.Equal(t, "team", other["billing_source"])
		assert.EqualValues(t, team.Id, other["team_id"])
		assert.Contains(t, other, "model_ratio", "price snapshot must survive independent log delivery")
		assert.Contains(t, other, "group_ratio")
		loggedQuota += int64(log.Quota)
	}
	assert.Equal(t, totalQuota, loggedQuota)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, owner.Id).Error)
	assert.EqualValues(t, 1000, stored.Quota, "team requests must not debit the personal wallet")
	// The upstream succeeds with a higher actual charge than the reservation.
	// The client must not receive that successful body while the shared period
	// cannot settle the difference; recovery later charges exactly once.
	var period model.TeamQuotaPeriod
	require.NoError(t, model.DB.First(&period, usages[0].PeriodId).Error)
	require.NoError(t, model.DB.Model(&period).Update("amount_total", period.AmountUsed+1000).Error)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/alpha/search", strings.NewReader(`{"model":"gpt-4o-mini","query":"ping"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token.Key)
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "pong")
	var pending model.TeamUsage
	require.NoError(t, model.DB.Where("team_id = ? AND status = ? AND pending_final = ?", team.Id, "reserved", true).First(&pending).Error)
	assert.Greater(t, pending.PendingFinalAmount, pending.Reserved)
	status, err := model.GetTeamOwnRequestStatus(pending.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "withheld_pending_settlement", status.ResponseState)
	require.NoError(t, model.DB.Model(&period).Update("amount_total", 1000000).Error)
	recovered, err := model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	recovered, err = model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered)
	require.NoError(t, model.DB.Where("request_id = ?", pending.RequestId).First(&pending).Error)
	assert.Equal(t, "settled", pending.Status)
	status, err = model.GetTeamOwnRequestStatus(pending.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "unconfirmed", status.ResponseState,
		"recovery of the charge must not claim that the HTTP response was delivered")
	assert.Equal(t, pending.PendingFinalAmount, pending.FinalAmount)
	require.NoError(t, service.DeliverPendingTeamSyncBillingEvents(request.Context(), 100))
	var recoveredLogs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", pending.RequestId).Count(&recoveredLogs).Error)
	assert.EqualValues(t, 1, recoveredLogs)
	require.NoError(t, model.DB.First(&stored, owner.Id).Error)
	assert.EqualValues(t, 1000, stored.Quota)
}

// The production credential gate stays closed; this local route exercises the
// real distributor after live membership validation, without making a billable
// upstream request or granting a team access to the owner's other groups.
func TestTeamTokenDistributorSelectsMemberGroupAndRevokesRemovedMember(t *testing.T) {
	setupRelayRouterTestDB(t)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Team{}, &model.TeamMember{},
		&model.TeamInvitation{}, &model.TeamSubscription{}, &model.TeamOrder{}, &model.BusinessEvent{}))
	owner := &model.User{Username: "team-distributor-owner", Email: "distributor-owner@example.test",
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000, AffCode: "distributor-owner"}
	member := &model.User{Username: "team-distributor-member", Email: "distributor-member@example.test",
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000, AffCode: "distributor-member"}
	require.NoError(t, model.DB.Create(owner).Error)
	require.NoError(t, model.DB.Create(member).Error)
	team, err := model.CreateTeam(owner.Id, "Local distributor test")
	require.NoError(t, err)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TeamSubscription{TeamId: team.Id, SeatLimit: 2,
		AmountTotal: 1000, ResetPeriod: model.SubscriptionResetNever,
		StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}).Error)
	invite, err := model.InviteTeamMember(owner.Id, member.Email)
	require.NoError(t, err)
	require.NoError(t, model.AcceptTeamInvitation(member.Id, invite.Id))
	token, err := model.CreateTeamToken(member.Id, "Member distributor", "default", "local-model")
	require.NoError(t, err)
	for _, group := range []string{"default", "vip"} {
		channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "local-" + group,
			Key: "local-test-only", Status: common.ChannelStatusEnabled, Models: "local-model", Group: group}
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: "local-model",
			ChannelId: channel.Id, Enabled: true, Weight: 100}).Error)
	}
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		validated, err := model.ValidateTeamToken(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		user, err := model.GetUserCache(validated.UserId)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		user.WriteContext(c)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, validated.Group)
		if err := middleware.SetupContextForToken(c, validated); err != nil {
			return
		}
		c.Next()
	}, middleware.Distribute(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"channel": common.GetContextKeyString(c, constant.ContextKeyChannelName),
			"group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup)})
	})
	request := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
			strings.NewReader(`{"model":"local-model","messages":[{"role":"user","content":"ping"}]}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token.Key)
		engine.ServeHTTP(recorder, r)
		return recorder
	}
	first := request()
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"channel":"local-default"`)
	assert.Contains(t, first.Body.String(), `"group":"default"`)
	require.NoError(t, model.RemoveTeamMember(owner.Id, member.Id))
	assert.Equal(t, http.StatusUnauthorized, request().Code, "membership revocation must block selection immediately")
}
