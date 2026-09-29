package model

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func teamTestFixture(t *testing.T) (*Team, *User, *User) {
	t.Helper()
	if !DB.Migrator().HasTable(&TeamUsage{}) {
		require.NoError(t, DB.AutoMigrate(&Team{}, &TeamMember{}, &TeamInvitation{}, &TeamSubscription{}, &TeamQuotaPeriod{}, &TeamOrder{}, &TeamUsage{}, &TeamSyncBillingEvent{}, &TeamSyncLogReceipt{}, &TeamTaskBillingEvent{}, &TeamTaskLogReceipt{}))
	}
	require.NoError(t, DB.AutoMigrate(&TeamUsage{}, &TeamSyncBillingEvent{}, &TeamSyncLogReceipt{}))
	require.NoError(t, DB.AutoMigrate(&TeamTaskBillingEvent{}, &TeamTaskLogReceipt{}))
	owner := &User{Username: fmt.Sprintf("team-owner-%s", t.Name()), Email: fmt.Sprintf("owner-%s@example.test", t.Name()), Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	target := &User{Username: fmt.Sprintf("team-target-%s", t.Name()), Email: fmt.Sprintf("target-%s@example.test", t.Name()), Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	require.NoError(t, DB.Create(owner).Error)
	require.NoError(t, DB.Create(target).Error)
	team, err := CreateTeam(owner.Id, "Test team")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamUsage{}).Error
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamSyncBillingEvent{}).Error
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamTaskBillingEvent{}).Error
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamOrder{}).Error
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamInvitation{}).Error
		var subscriptions []TeamSubscription
		_ = DB.Where("team_id = ?", team.Id).Find(&subscriptions).Error
		for _, sub := range subscriptions {
			_ = DB.Where("team_subscription_id = ?", sub.Id).Delete(&TeamQuotaPeriod{}).Error
		}
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamSubscription{}).Error
		_ = DB.Where("team_id = ?", team.Id).Delete(&TeamMember{}).Error
		_ = DB.Delete(team).Error
		_ = DB.Delete(owner).Error
		_ = DB.Delete(target).Error
	})
	return team, owner, target
}

func TestTeamInvitationRequiresActiveSubscriptionAndRespectsScheduledSeatLimit(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	_, err := InviteTeamMember(owner.Id, target.Email)
	require.Error(t, err)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now + 3600, EndTime: now + 7200, Status: TeamStatusActive}).Error)
	_, err = InviteTeamMember(owner.Id, target.Email)
	require.Error(t, err, "a scheduled term must not allow invitations before activation")

	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	invite, err := InviteTeamMember(owner.Id, target.Email)
	require.NoError(t, err)
	require.NoError(t, AcceptTeamInvitation(target.Id, invite.Id))
	assert.Error(t, AcceptTeamInvitation(target.Id, invite.Id), "an invitation must be accepted only once")
}

func TestPendingTeamPaymentKeepsSeatLimitAfterThirtyMinutes(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 3, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	pending := &TeamOrder{TeamId: team.Id, PayerUserId: owner.Id, SeatLimit: 2, Status: common.TopUpStatusPending,
		TradeNo: "older-pending-" + t.Name(), CreateTime: now - 3600}
	require.NoError(t, DB.Create(pending).Error)
	target := &User{Username: "pending-seat-" + t.Name(), Email: "pending-seat-" + t.Name() + "@example.test",
		Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	require.NoError(t, DB.Create(target).Error)
	t.Cleanup(func() { _ = DB.Delete(target).Error })
	first, err := InviteTeamMember(owner.Id, target.Email)
	require.NoError(t, err)
	require.NotNil(t, first)
	other := &User{Username: "pending-other-" + t.Name(), Email: "pending-other-" + t.Name() + "@example.test",
		Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	require.NoError(t, DB.Create(other).Error)
	t.Cleanup(func() { _ = DB.Delete(other).Error })
	_, err = InviteTeamMember(owner.Id, other.Email)
	assert.Error(t, err, "a late signed callback must not activate a term with excess seats")
}

func TestTeamQuotaReservationSettlementAndRefundStayInOriginalPeriod(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	sub := &TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}
	require.NoError(t, DB.Create(sub).Error)
	usage, period, err := reserveTeamQuota("team-reserve-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	assert.EqualValues(t, 70, period.AmountUsed)
	_, _, err = reserveTeamQuota("team-second-"+t.Name(), team.Id, owner.Id, 0, 31)
	require.Error(t, err)
	_, _, err = reserveTeamQuota(usage.RequestId, team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	require.NoError(t, AdjustTeamUsage(usage.RequestId, -20, true))
	require.NoError(t, AdjustTeamUsage(usage.RequestId, 0, true))
	assert.Error(t, RefundTeamUsage(usage.RequestId), "settled usage cannot be refunded twice")
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 50, period.AmountUsed)

	second, _, err := reserveTeamQuota("team-refund-"+t.Name(), team.Id, owner.Id, 0, 40)
	require.NoError(t, err)
	require.NoError(t, RefundTeamUsage(second.RequestId))
	require.NoError(t, RefundTeamUsage(second.RequestId))
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 50, period.AmountUsed)
}

func TestTeamReservationRechecksTokenMembershipAndSubscription(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	now := common.GetTimestamp()
	sub := &TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}
	require.NoError(t, DB.Create(sub).Error)
	token, err := CreateTeamToken(owner.Id, "Live credential", "default", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(token).Error })

	usage, _, err := PreConsumeTeamForToken("live-team-token-"+t.Name(), team.Id, owner.Id, token.Id, 20)
	require.NoError(t, err)
	_, _, err = PreConsumeTeamForToken(usage.RequestId, team.Id, owner.Id, token.Id, 20)
	require.Error(t, err, "a duplicate handler must not own or refund the first handler's reservation")
	require.NoError(t, RefundTeamUsage(usage.RequestId))
	require.NoError(t, DisableTeamToken(owner.Id, team.Id, token.Id))
	_, _, err = PreConsumeTeamForToken("disabled-team-token-"+t.Name(), team.Id, owner.Id, token.Id, 20)
	require.ErrorIs(t, err, ErrTokenInvalid)

	member := &TeamMember{TeamId: team.Id, UserId: target.Id, Role: "member", JoinedAt: now}
	require.NoError(t, DB.Create(member).Error)
	memberToken, err := CreateTeamToken(target.Id, "Member credential", "default", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(memberToken).Error })
	require.NoError(t, RemoveTeamMember(owner.Id, target.Id))
	_, _, err = PreConsumeTeamForToken("removed-member-"+t.Name(), team.Id, target.Id, memberToken.Id, 20)
	require.Error(t, err)

	activeToken, err := CreateTeamToken(owner.Id, "Expiring credential", "default", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(activeToken).Error })
	require.NoError(t, DB.Model(sub).Update("end_time", now-1).Error)
	_, _, err = PreConsumeTeamForToken("expired-team-sub-"+t.Name(), team.Id, owner.Id, activeToken.Id, 20)
	require.Error(t, err)
	var count int64
	require.NoError(t, DB.Model(&TeamUsage{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count, "rejected requests must not create reservations")
}

func TestTeamRequestStatusIsScopedToCallingMember(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, _, err := reserveTeamQuota("request-status-"+t.Name(), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	_, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, target.Id)
	require.Error(t, err, "another account must not inspect request accounting")
	_, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id+1, owner.Id)
	require.Error(t, err, "an old team request must not leak into a future membership")
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 40))
	status, err := GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending_settlement", status.Status)
	assert.EqualValues(t, 40, status.PendingAmount)
	assert.Equal(t, "withheld_pending_settlement", status.ResponseState)
	require.NoError(t, AdjustTeamUsage(usage.RequestId, 10, true))
	status, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "settled", status.Status)
	assert.EqualValues(t, 40, status.FinalAmount)
	assert.Zero(t, status.PendingAmount)
	assert.Equal(t, "unconfirmed", status.ResponseState)
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	assert.Error(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	status, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "writing", status.ResponseState)
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "write_failed"))
	status, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "write_failed", status.ResponseState)
	assert.Error(t, MarkTeamSyncResponseState(usage.RequestId, "server_write_completed"),
		"an interrupted write cannot later be reported as delivered")
}

func TestFormerMemberCanAuditOwnPendingChargeWithoutSeeingOtherMember(t *testing.T) {
	team, owner, member := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	invitation, err := InviteTeamMember(owner.Id, member.Email)
	require.NoError(t, err)
	require.NoError(t, AcceptTeamInvitation(member.Id, invitation.Id))
	usage, _, err := reserveTeamQuota(fmt.Sprintf("former-member-%d", team.Id), team.Id, member.Id, 0, 60)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 80))
	require.NoError(t, RemoveTeamMember(owner.Id, member.Id))
	_, _, err = GetMyTeam(member.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	status, err := GetTeamMemberRequestStatus(usage.RequestId, member.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending_settlement", status.Status)
	assert.EqualValues(t, 80, status.PendingAmount)
	assert.Equal(t, "withheld_pending_settlement", status.ResponseState)
	_, err = GetTeamMemberRequestStatus(usage.RequestId, owner.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	memberRequests, err := ListTeamMemberRequestStatuses(member.Id, 50, "")
	require.NoError(t, err)
	require.NotEmpty(t, memberRequests)
	assert.Equal(t, usage.RequestId, memberRequests[0].RequestId)
	assert.Equal(t, "pending_settlement", memberRequests[0].Status)
	ownerRequests, err := ListTeamMemberRequestStatuses(owner.Id, 50, "")
	require.NoError(t, err)
	for _, request := range ownerRequests {
		assert.NotEqual(t, usage.RequestId, request.RequestId)
	}
}

func TestTeamRecentRequestStatusesFindsChargedResponseWithoutHeader(t *testing.T) {
	team, owner, other := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	earlier, _, err := reserveTeamQuota(fmt.Sprintf("earlier-status-%d", team.Id), team.Id, owner.Id, 0, 10)
	require.NoError(t, err)
	usage, _, err := reserveTeamQuota(fmt.Sprintf("lost-header-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 40))
	require.NoError(t, SettleTeamSyncUsage(usage.RequestId, 40))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "write_failed"))
	requests, err := ListTeamMemberRequestStatuses(owner.Id, 50, "")
	require.NoError(t, err)
	require.NotEmpty(t, requests)
	assert.Equal(t, usage.RequestId, requests[0].RequestId)
	assert.Equal(t, "settled", requests[0].Status)
	assert.Equal(t, "write_failed", requests[0].ResponseState)
	assert.EqualValues(t, 40, requests[0].FinalAmount)
	otherRequests, err := ListTeamMemberRequestStatuses(other.Id, 50, "")
	require.NoError(t, err)
	for _, request := range otherRequests {
		assert.NotEqual(t, usage.RequestId, request.RequestId)
	}
	_, err = ListTeamMemberRequestStatuses(owner.Id, 101, "")
	require.Error(t, err)
	_, err = ListTeamMemberRequestStatuses(other.Id, 50, usage.RequestId)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a foreign cursor must not reveal another member's request")
	older, err := ListTeamMemberRequestStatuses(owner.Id, 50, usage.RequestId)
	require.NoError(t, err)
	require.Len(t, older, 1)
	assert.Equal(t, earlier.RequestId, older[0].RequestId)
}

func TestTeamSettledResponseWithheldBeforeNetworkWrite(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, _, err := reserveTeamQuota(fmt.Sprintf("withheld-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 40))
	require.NoError(t, SettleTeamSyncUsage(usage.RequestId, 40))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "withheld"))
	status, err := GetTeamMemberRequestStatus(usage.RequestId, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "settled", status.Status)
	assert.EqualValues(t, 40, status.FinalAmount)
	assert.Equal(t, "withheld", status.ResponseState)
	assert.Error(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	assert.Error(t, MarkTeamSyncResponseState(usage.RequestId, "server_write_completed"))
}

func TestTeamUpstreamAttemptSurvivesMissingOutcomeAndCannotBeReopened(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, _, err := reserveTeamQuota("unknown-upstream-crash", team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	status, err := GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "reserved", status.Status)
	audits, err := PendingTeamRequestAudits(100)
	require.NoError(t, err)
	assert.Empty(t, audits, "a reservation not forwarded upstream is not an ambiguous result")
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.Error(t, MarkTeamSyncUpstreamAttempt(usage.RequestId), "another relay invocation must not send the same request twice")
	status, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "upstream_outcome_unconfirmed", status.Status)
	assert.Equal(t, "not_recorded", status.ResponseState)
	audits, err = PendingTeamRequestAudits(100)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	assert.Equal(t, "upstream_outcome_unconfirmed", audits[0].Status)
	assert.Equal(t, owner.Id, audits[0].MemberUserId)
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 40))
	_, err = ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, "provider-case:confirmed")
	require.Error(t, err, "a durable successful charge cannot be marked as a failed upstream response")
	require.Error(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	status, err = GetTeamOwnRequestStatus(usage.RequestId, team.Id, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending_settlement", status.Status)
	audits, err = PendingTeamRequestAudits(100)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	assert.Equal(t, "pending_settlement", audits[0].Status)
	assert.Equal(t, usage.RequestId, audits[0].RequestId)
	assert.Equal(t, "withheld_pending_settlement", audits[0].ResponseState)
	assert.EqualValues(t, 40, audits[0].PendingAmount)
	require.NoError(t, AdjustTeamUsage(usage.RequestId, 10, true))
	audits, err = PendingTeamRequestAudits(100)
	require.NoError(t, err)
	assert.Empty(t, audits, "settled requests leave the pending queue")
}

func TestTeamAmbiguousAttemptRequiresEvidenceBeforeRefund(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota("unknown-needs-review", team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.Error(t, RefundTeamUsage(usage.RequestId))
	for _, bad := range []string{"", "short", "ticket with secrets", "https://provider.example/secret"} {
		_, err = ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, bad)
		require.Error(t, err)
	}
	applied, err := ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, "provider-case:12345")
	require.NoError(t, err)
	assert.True(t, applied)
	applied, err = ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, "provider-case:12345")
	require.NoError(t, err)
	assert.False(t, applied)
	_, err = ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, "provider-case:changed")
	require.Error(t, err)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.Zero(t, period.AmountUsed)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "refunded", usage.Status)
	assert.Equal(t, "provider-case:12345", usage.EvidenceRef)
	assert.Equal(t, owner.Id, usage.ResolvedBy)
	assert.Positive(t, usage.ResolvedAt)
	var eventCount int64
	require.NoError(t, DB.Model(&TeamSyncBillingEvent{}).Where("request_id = ?", usage.RequestId).Count(&eventCount).Error)
	assert.Zero(t, eventCount, "documented failed result must not create a consumption event")
}

func TestProviderConfirmedTeamChargeReconcilesOriginalPeriodOnceWithoutInventingDelivery(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota("provider-billed", team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	other, _, err := reserveTeamQuota("provider-other", team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	for _, amount := range []int64{-1, common.MaxQuota + 1} {
		_, err := ResolveTeamAttemptAsBilled(usage.RequestId, owner.Id, "provider-charge:12345", amount)
		require.Error(t, err)
	}
	applied, err := ResolveTeamAttemptAsBilled(usage.RequestId, owner.Id, "provider-charge:12345", 90)
	require.NoError(t, err)
	assert.True(t, applied)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "reserved", usage.Status)
	assert.True(t, usage.PendingFinal)
	assert.Error(t, RefundTeamUsage(usage.RequestId))
	_, err = ResolveTeamAttemptAsFailed(usage.RequestId, owner.Id, "provider-charge:12345")
	require.Error(t, err)
	_, err = ResolveTeamAttemptAsBilled(usage.RequestId, owner.Id, "provider-charge:12345", 80)
	require.Error(t, err)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", usage.RequestId).Count(&count).Error)
	assert.Zero(t, count, "insufficient quota cannot produce a charged log")
	require.NoError(t, RefundTeamUsage(other.RequestId))
	recovered, err := RecoverPendingTeamSettlements(1)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	applied, err = ResolveTeamAttemptAsBilled(usage.RequestId, owner.Id, "provider-charge:12345", 90)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.EqualValues(t, 90, usage.FinalAmount)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 90, period.AmountUsed)
	var event TeamSyncBillingEvent
	require.NoError(t, DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.Equal(t, "withheld", event.ResponseState)
	assert.True(t, event.Ready)
	for i := 0; i < 2; i++ {
		require.NoError(t, DeliverTeamSyncBillingLog(context.Background(), event))
	}
	var logs []Log
	require.NoError(t, LOG_DB.Where("request_id = ?", usage.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Other, `"pricing_breakdown":"not_recorded"`)
	assert.Contains(t, logs[0].Other, `"evidence_ref":"provider-charge:12345"`)
	assert.Equal(t, 90, logs[0].Quota)
	formatUserLogs([]*Log{&logs[0]}, 0)
	assert.NotContains(t, logs[0].Other, "evidence_ref", "provider audit reference must stay admin-only")
	assert.Contains(t, logs[0].Other, "manual_reconciliation")
	var user User
	require.NoError(t, DB.First(&user, owner.Id).Error)
	assert.Equal(t, owner.Quota, user.Quota)
	assert.EqualValues(t, 90, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	status, err := GetTeamMemberRequestStatus(usage.RequestId, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "withheld", status.ResponseState)
}

func TestTeamSyncSettlementIntentSurvivesInsufficientQuotaAndBlocksRefund(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, _, err := reserveTeamQuota("sync-intent-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	other, _, err := reserveTeamQuota("sync-other-"+t.Name(), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 90))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 90))
	assert.Error(t, RecordTeamSyncSettlementIntent(usage.RequestId, 80), "upstream cost cannot be replaced on retry")
	assert.Error(t, AdjustTeamUsage(usage.RequestId, 20, true), "another outstanding reservation must not be overdrawn")
	assert.Error(t, RefundTeamUsage(usage.RequestId), "upstream success is not an abandoned reservation")
	var reloaded TeamUsage
	require.NoError(t, DB.Where("request_id = ?", usage.RequestId).First(&reloaded).Error)
	assert.True(t, reloaded.PendingFinal)
	assert.EqualValues(t, 90, reloaded.PendingFinalAmount)
	assert.Equal(t, "reserved", reloaded.Status)
	assert.Error(t, AdjustTeamUsage(usage.RequestId, 10, true), "replayed settlement must match the recorded cost")
	recovered, err := RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered, "recovery cannot overdraw the period while another request is outstanding")
	require.NoError(t, RefundTeamUsage(other.RequestId))
	recovered, err = RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	recovered, err = RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered, "recovery must never double charge")
	require.NoError(t, DB.First(&reloaded, reloaded.Id).Error)
	assert.Equal(t, "settled", reloaded.Status)
	assert.EqualValues(t, 90, reloaded.FinalAmount)
	assert.False(t, reloaded.PendingFinal)
	require.NoError(t, RecordTeamSyncSettlementIntent(reloaded.RequestId, 90))
	assert.Error(t, RecordTeamSyncSettlementIntent(reloaded.RequestId, 80))
}

func TestPendingTeamAuditShowsOriginalPeriodShortfall(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota("audit-shortfall-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	other, _, err := reserveTeamQuota("audit-other-"+t.Name(), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 90))
	audits, err := PendingTeamRequestAudits(100)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	assert.EqualValues(t, 20, audits[0].Shortfall)
	assert.Equal(t, period.EndTime, audits[0].PeriodEndTime)
	assert.Equal(t, "withheld_pending_settlement", audits[0].ResponseState)
	require.NoError(t, RefundTeamUsage(other.RequestId))
	audits, err = PendingTeamRequestAudits(100)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	assert.Zero(t, audits[0].Shortfall)
	assert.EqualValues(t, 90, audits[0].PendingAmount)
}

func TestUndeliveredTeamChargeCanBeExplicitlyAbsorbedWithoutChargingMember(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota(fmt.Sprintf("compensated-%d", team.Id), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	other, _, err := reserveTeamQuota(fmt.Sprintf("compensation-other-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 90))
	_, _, err = CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "bad")
	require.Error(t, err)
	applied, cost, err := CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "provider:loss-123")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.EqualValues(t, 90, cost)
	applied, cost, err = CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "provider:loss-123")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.EqualValues(t, 90, cost)
	_, _, err = CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "provider:changed-123")
	require.Error(t, err)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 30, period.AmountUsed, "only the other live reservation remains")
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "refunded", usage.Status)
	assert.False(t, usage.PendingFinal)
	assert.EqualValues(t, 0, usage.FinalAmount)
	var event TeamSyncBillingEvent
	require.NoError(t, DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.Equal(t, "compensated_no_delivery", event.ResponseState)
	assert.EqualValues(t, 90, event.FinalQuota, "retain calculated unbilled quota for audit")
	assert.False(t, event.Ready, "provider loss must not become a member charge log")
	assert.Contains(t, event.BillingDetails, "provider:loss-123")
	status, err := GetTeamMemberRequestStatus(usage.RequestId, owner.Id)
	require.NoError(t, err)
	assert.Equal(t, "compensated_no_delivery", status.ResponseState)
	assert.EqualValues(t, 90, status.UnbilledQuota)
	assert.EqualValues(t, 0, status.FinalAmount)
	var stored User
	require.NoError(t, DB.First(&stored, owner.Id).Error)
	assert.Equal(t, owner.Quota, stored.Quota)
	assert.Equal(t, owner.UsedQuota, stored.UsedQuota)
	assert.Equal(t, owner.RequestCount, stored.RequestCount)
	recovered, err := RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered)
	require.NoError(t, RefundTeamUsage(other.RequestId))
	assert.Error(t, SettleTeamSyncUsage(usage.RequestId, 90))
}

func TestTeamCompensationRejectsAffordableOrPossiblyDeliveredResult(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota(fmt.Sprintf("reject-comp-%d", team.Id), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 90))
	_, _, err = CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "provider:reject-123")
	require.ErrorContains(t, err, "can settle")
	other, _, err := reserveTeamQuota(fmt.Sprintf("reject-other-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&TeamSyncBillingEvent{}).Where("request_id = ?", usage.RequestId).
		Update("response_state", "writing").Error)
	_, _, err = CompensateTeamUndeliveredCharge(usage.RequestId, owner.Id, "provider:reject-123")
	require.ErrorContains(t, err, "not eligible")
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 100, period.AmountUsed)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "reserved", usage.Status)
	assert.True(t, usage.PendingFinal)
	// The other pre-consume still belongs to its original request.
	require.NoError(t, RefundTeamUsage(other.RequestId))
}

func TestSettledDeliveryDisputeCreditsOriginalPeriodExactlyOnceWithoutErasingUsage(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota(fmt.Sprintf("settled-dispute-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 40))
	_, _, err = CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:too-early")
	require.Error(t, err, "an unsettled response cannot receive a settled charge credit")
	require.NoError(t, SettleTeamSyncUsage(usage.RequestId, 40))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "write_failed"))
	applied, credited, err := CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:disputed-123")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.EqualValues(t, 40, credited)
	applied, credited, err = CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:disputed-123")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.EqualValues(t, 40, credited)
	require.NoError(t, DB.Model(period).Update("end_time", now-1).Error)
	applied, credited, err = CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:disputed-123")
	require.NoError(t, err, "reading an earlier credit must remain idempotent after expiry")
	assert.False(t, applied)
	assert.EqualValues(t, 40, credited)
	_, _, err = CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:different-123")
	require.Error(t, err, "a different review cannot issue a second credit")
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.Zero(t, period.AmountUsed)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.EqualValues(t, 40, usage.FinalAmount, "provider usage remains gross")
	var event TeamSyncBillingEvent
	require.NoError(t, DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.True(t, event.Ready, "the original consume log still has to be delivered")
	assert.EqualValues(t, 40, event.FinalQuota)
	assert.EqualValues(t, 40, event.CreditedQuota)
	assert.Equal(t, "write_failed", event.ResponseState)
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}, &TeamSyncLogReceipt{}))
	require.NoError(t, DeliverTeamSyncBillingLog(context.Background(), event))
	require.NoError(t, DeliverTeamSyncBillingLog(context.Background(), event))
	var logs []Log
	require.NoError(t, LOG_DB.Where("request_id = ?", usage.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1, "the delivery credit must not create a second consume log")
	assert.Equal(t, 40, logs[0].Quota, "provider usage remains gross in the historical consume log")
	t.Cleanup(func() {
		_ = LOG_DB.Where("request_id = ?", usage.RequestId).Delete(&Log{}).Error
		_ = LOG_DB.Where("request_id = ?", usage.RequestId).Delete(&TeamSyncLogReceipt{}).Error
	})
	status, err := GetTeamMemberRequestStatus(usage.RequestId, owner.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 40, status.CreditedQuota)
	assert.Equal(t, "write_failed", status.ResponseState)
	periodCredit, err := TeamPeriodCreditedQuota(period.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 40, periodCredit)
	var stored User
	require.NoError(t, DB.First(&stored, owner.Id).Error)
	assert.EqualValues(t, 40, stored.UsedQuota, "actual model usage remains recorded")
	assert.Equal(t, owner.Quota, stored.Quota, "no personal-wallet adjustment")
}

func TestSettledDeliveryDisputeCannotCreditExpiredQuotaPeriod(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 3600, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota(fmt.Sprintf("expired-dispute-%d", team.Id), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	require.NoError(t, MarkTeamSyncUpstreamAttempt(usage.RequestId))
	require.NoError(t, RecordTeamSyncSettlementIntent(usage.RequestId, 30))
	require.NoError(t, SettleTeamSyncUsage(usage.RequestId, 30))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "writing"))
	require.NoError(t, MarkTeamSyncResponseState(usage.RequestId, "write_failed"))
	// A reset or expiry makes the original period unusable, even when the
	// subscription itself is still active. No credit moves to the new period.
	require.NoError(t, DB.Model(period).Update("end_time", now-1).Error)
	applied, credited, err := CreditTeamDeliveryDispute(usage.RequestId, owner.Id, "case:expired-123")
	require.ErrorContains(t, err, "period has expired")
	assert.False(t, applied)
	assert.Zero(t, credited)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 30, period.AmountUsed)
	var event TeamSyncBillingEvent
	require.NoError(t, DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.Zero(t, event.CreditedQuota)
}

func TestTeamRecoveryUnderfundedRequestDoesNotBlockAnotherTeam(t *testing.T) {
	blockedTeam, blockedOwner, readyOwner := teamTestFixture(t)
	readyTeam, err := CreateTeam(readyOwner.Id, "Ready recovery team")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = DB.Where("team_id = ?", readyTeam.Id).Delete(&TeamUsage{}).Error
		_ = DB.Where("team_id = ?", readyTeam.Id).Delete(&TeamSyncBillingEvent{}).Error
		_ = DB.Where("team_id = ?", readyTeam.Id).Delete(&TeamSubscription{}).Error
		_ = DB.Where("team_id = ?", readyTeam.Id).Delete(&TeamMember{}).Error
		_ = DB.Delete(readyTeam).Error
	})
	now := common.GetTimestamp()
	for _, team := range []*Team{blockedTeam, readyTeam} {
		require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
			ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	}
	blocked, _, err := reserveTeamQuota(fmt.Sprintf("blocked-%d", blockedTeam.Id), blockedTeam.Id, blockedOwner.Id, 0, 100)
	require.NoError(t, err)
	require.NoError(t, RecordTeamSyncSettlementIntent(blocked.RequestId, 110))
	ready, _, err := reserveTeamQuota(fmt.Sprintf("ready-%d", readyTeam.Id), readyTeam.Id, readyOwner.Id, 0, 30)
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(&TeamQuotaPeriod{}, ready.PeriodId).Error })
	require.NoError(t, RecordTeamSyncSettlementIntent(ready.RequestId, 40))
	recovered, err := RecoverPendingTeamSettlements(1)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	require.NoError(t, DB.First(blocked, blocked.Id).Error)
	assert.Equal(t, "reserved", blocked.Status)
	assert.True(t, blocked.PendingFinal)
	require.NoError(t, DB.First(ready, ready.Id).Error)
	assert.Equal(t, "settled", ready.Status)
	assert.EqualValues(t, 40, ready.FinalAmount)
	var user User
	require.NoError(t, DB.First(&user, readyOwner.Id).Error)
	assert.EqualValues(t, 1, user.RequestCount)
	assert.Equal(t, readyOwner.Quota, user.Quota)
}

func TestTeamTaskFinalizationRetriesAfterInsufficientSettlementWithoutLosingReservation(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	channel := &Channel{Name: "team-finalize-" + t.Name()}
	require.NoError(t, DB.Create(channel).Error)
	t.Cleanup(func() { _ = DB.Delete(channel).Error })
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, _, err := reserveTeamQuota("team-finalize-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	other, _, err := reserveTeamQuota("team-other-"+t.Name(), team.Id, owner.Id, 0, 30)
	require.NoError(t, err)
	task := &Task{TaskID: "team-finalize-" + t.Name(), UserId: owner.Id, ChannelId: channel.Id, Quota: 70,
		Status: TaskStatusInProgress, Progress: "50%", PrivateData: TaskPrivateData{
			BillingSource: "team", TeamId: team.Id, TeamRequestId: usage.RequestId,
		}}
	require.NoError(t, DB.Create(task).Error)
	t.Cleanup(func() { _ = DB.Delete(task).Error })
	final := *task
	final.Status = TaskStatusSuccess
	final.Progress = "100%"
	final.Quota = 90
	_, err = FinalizeTeamTask(&final, TaskStatusInProgress, 90, "completion", nil)
	require.Error(t, err)
	require.NoError(t, DB.First(task, task.ID).Error)
	assert.Equal(t, TaskStatus(TaskStatusInProgress), task.Status)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "reserved", usage.Status)
	assert.EqualValues(t, 70, usage.Reserved)

	require.NoError(t, RefundTeamUsage(other.RequestId))
	won, err := FinalizeTeamTask(&final, TaskStatusInProgress, 90, "completion", nil)
	require.NoError(t, err)
	assert.True(t, won)
	won, err = FinalizeTeamTask(&final, TaskStatusInProgress, 90, "completion", nil)
	require.NoError(t, err)
	assert.False(t, won)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.EqualValues(t, 90, usage.FinalAmount)
	var updatedOwner User
	var updatedChannel Channel
	require.NoError(t, DB.First(&updatedOwner, owner.Id).Error)
	require.NoError(t, DB.First(&updatedChannel, channel.Id).Error)
	assert.Equal(t, owner.UsedQuota+20, updatedOwner.UsedQuota)
	assert.Equal(t, channel.UsedQuota+20, updatedChannel.UsedQuota)
}

func TestTeamTaskFinalizationRefundsOnceWithoutTouchingMemberWallet(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	channel := &Channel{Name: "team-failed-" + t.Name(), UsedQuota: 70}
	require.NoError(t, DB.Create(channel).Error)
	t.Cleanup(func() { _ = DB.Delete(channel).Error })
	require.NoError(t, DB.Model(owner).Update("used_quota", 70).Error)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota("team-failed-finalize-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	task := &Task{TaskID: "team-failed-finalize-" + t.Name(), UserId: owner.Id, ChannelId: channel.Id, Quota: 70,
		Status: TaskStatusSubmitted, PrivateData: TaskPrivateData{BillingSource: "team", TeamId: team.Id, TeamRequestId: usage.RequestId}}
	require.NoError(t, DB.Create(task).Error)
	t.Cleanup(func() { _ = DB.Delete(task).Error })
	failed := *task
	failed.Status = TaskStatusFailure
	failed.FailReason = "upstream failed"
	won, err := FinalizeTeamTask(&failed, TaskStatusSubmitted, 0, "failure", nil)
	require.NoError(t, err)
	assert.True(t, won)
	won, err = FinalizeTeamTask(&failed, TaskStatusSubmitted, 0, "failure", nil)
	require.NoError(t, err)
	assert.False(t, won)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "refunded", usage.Status)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.Zero(t, period.AmountUsed)
	var member User
	require.NoError(t, DB.First(&member, owner.Id).Error)
	assert.Equal(t, owner.Quota, member.Quota)
	assert.Zero(t, member.UsedQuota)
	require.NoError(t, DB.First(channel, channel.Id).Error)
	assert.Zero(t, channel.UsedQuota)
}

func TestTeamTaskFinalizationCounterFailureRollsBackQuotaAndStatus(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	usage, period, err := reserveTeamQuota("team-counter-rollback-"+t.Name(), team.Id, owner.Id, 0, 70)
	require.NoError(t, err)
	task := &Task{TaskID: "team-counter-rollback-" + t.Name(), UserId: owner.Id, ChannelId: 99999999,
		Quota: 70, Status: TaskStatusInProgress, PrivateData: TaskPrivateData{
			BillingSource: "team", TeamId: team.Id, TeamRequestId: usage.RequestId,
		}}
	require.NoError(t, DB.Create(task).Error)
	t.Cleanup(func() { _ = DB.Delete(task).Error })
	final := *task
	final.Status = TaskStatusSuccess
	won, err := FinalizeTeamTask(&final, TaskStatusInProgress, 90, "completion", nil)
	require.Error(t, err)
	assert.False(t, won)
	require.NoError(t, DB.First(task, task.ID).Error)
	assert.Equal(t, TaskStatus(TaskStatusInProgress), task.Status)
	require.NoError(t, DB.First(usage, usage.Id).Error)
	assert.Equal(t, "reserved", usage.Status)
	require.NoError(t, DB.First(period, period.Id).Error)
	assert.EqualValues(t, 70, period.AmountUsed)
	var updatedOwner User
	require.NoError(t, DB.First(&updatedOwner, owner.Id).Error)
	assert.Equal(t, owner.UsedQuota, updatedOwner.UsedQuota)
}

func TestMemberUsageExcludesOtherMembersAndRefundedRequests(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	require.NoError(t, DB.Create(&TeamUsage{RequestId: "team-owner-usage-" + t.Name(), TeamId: team.Id,
		MemberUserId: owner.Id, Reserved: 80, FinalAmount: 70, Status: "settled"}).Error)
	require.NoError(t, DB.Create(&TeamUsage{RequestId: "team-member-usage-" + t.Name(), TeamId: team.Id,
		MemberUserId: target.Id, Reserved: 40, FinalAmount: 30, Status: "settled"}).Error)
	require.NoError(t, DB.Create(&TeamUsage{RequestId: "team-member-refund-" + t.Name(), TeamId: team.Id,
		MemberUserId: target.Id, Reserved: 40, Status: "refunded"}).Error)

	amount, requests, err := TeamOwnUsage(team.Id, target.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 30, amount)
	assert.EqualValues(t, 1, requests)
}

func TestConcurrentTeamReservationsCannotExceedOnePeriod(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, requestId := range []string{"concurrent-team-a-" + t.Name(), "concurrent-team-b-" + t.Name()} {
		wg.Add(1)
		go func(requestId string) {
			defer wg.Done()
			_, _, err := reserveTeamQuota(requestId, team.Id, owner.Id, 0, 60)
			results <- err
		}(requestId)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	assert.LessOrEqual(t, success, 1, "two simultaneous requests must not overdraw the shared quota")
	var used int64
	require.NoError(t, DB.Model(&TeamUsage{}).Where("team_id = ?", team.Id).Select("COALESCE(SUM(reserved), 0)").Scan(&used).Error)
	assert.EqualValues(t, int64(success)*60, used)
}

func TestTeamPaymentRejectsAmountAndProviderMismatchAndCompletesOnlyOnce(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	order := &TeamOrder{TeamId: team.Id, PayerUserId: owner.Id, PlanId: 12, PlanTitle: "Shared plan",
		SeatLimit: 2, AmountTotal: 500, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		ResetPeriod: SubscriptionResetNever, Money: 10.50, TradeNo: "team-pay-" + t.Name(),
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(order).Error)
	assert.Error(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "10.49", "alipay"))
	assert.Error(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "10.50", "wxpay"))
	var count int64
	require.NoError(t, DB.Model(&TeamSubscription{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "10.50", "alipay"))
	require.NoError(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "10.50", "alipay"))
	assert.Error(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-b", "10.50", "alipay"))
	require.NoError(t, DB.Model(&TeamSubscription{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	second := *order
	second.Id = 0
	second.TradeNo = "team-second-payment-" + t.Name()
	second.ProviderTradeNo = nil
	second.Status = common.TopUpStatusPending
	require.NoError(t, DB.Create(&second).Error)
	assert.Error(t, CompleteTeamPayment(second.TradeNo, PaymentProviderEpay, "provider-a", "10.50", "alipay"),
		"a provider transaction cannot purchase two periods")
	require.NoError(t, DB.Model(&TeamSubscription{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestPersonalBalanceCheckoutRejectsTeamPlanBeforeWalletDebit(t *testing.T) {
	_, owner, _ := teamTestFixture(t)
	plan := &SubscriptionPlan{Scope: "team", SeatLimit: 2, Title: "Shared only",
		PriceAmount: 10, TotalAmount: 500, Enabled: true, DurationUnit: SubscriptionDurationMonth, DurationValue: 1}
	require.NoError(t, DB.Create(plan).Error)
	t.Cleanup(func() { _ = DB.Delete(plan).Error })
	require.NoError(t, DB.Model(owner).Update("quota", 100000).Error)
	assert.Error(t, PurchaseSubscriptionWithBalance(owner.Id, plan.Id))
	var after User
	require.NoError(t, DB.First(&after, owner.Id).Error)
	assert.EqualValues(t, 100000, after.Quota)
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ? AND plan_id = ?", owner.Id, plan.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTeamCredentialStaysDisabledForLegacyAuthAndRevokesOnMembershipRemoval(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}).Error)
	invite, err := InviteTeamMember(owner.Id, target.Email)
	require.NoError(t, err)
	require.NoError(t, AcceptTeamInvitation(target.Id, invite.Id))
	token, err := CreateTeamToken(target.Id, "Member key", "", "model-a")
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(token).Error })
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	_, err = ValidateUserToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "previous images must not charge the member's wallet")
	_, err = ValidateTeamToken(token.Key)
	require.NoError(t, err)
	personalTokens, err := GetAllUserTokens(target.Id, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, personalTokens)
	require.NoError(t, RemoveTeamMember(owner.Id, target.Id))
	_, err = ValidateTeamToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestTeamCredentialRevokesWhenDisabledOrSubscriptionEnds(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	_, err := CreateTeamToken(owner.Id, "Before payment", "default", "")
	require.Error(t, err, "a team credential requires an active paid term")

	now := common.GetTimestamp()
	sub := &TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}
	require.NoError(t, DB.Create(sub).Error)
	token, err := CreateTeamToken(owner.Id, "Revocable key", "default", "model-a")
	require.NoError(t, err)
	t.Cleanup(func() { _ = DB.Delete(token).Error })
	_, err = ValidateTeamToken(token.Key)
	require.NoError(t, err)
	require.NoError(t, DB.Model(sub).Update("end_time", now-1).Error)
	_, err = ValidateTeamToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "expired subscriptions must revoke existing keys without a cache delay")
	require.NoError(t, DB.Model(sub).Update("end_time", now+3600).Error)
	_, err = ValidateTeamToken(token.Key)
	require.NoError(t, err)
	require.NoError(t, DisableTeamToken(owner.Id, team.Id, token.Id))
	_, err = ValidateTeamToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "disabled team keys must stop working immediately")
	assert.ErrorIs(t, DisableTeamToken(owner.Id, team.Id, token.Id), ErrTokenInvalid)
}

func TestTeamPaymentRejectsExpiredOrderAndWrongProviderWithoutNewTerm(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	order := &TeamOrder{TeamId: team.Id, PayerUserId: owner.Id, PlanId: 12, PlanTitle: "Shared plan",
		SeatLimit: 2, AmountTotal: 500, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		ResetPeriod: SubscriptionResetNever, Money: 10.50, TradeNo: "team-expired-" + t.Name(),
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(order).Error)
	assert.ErrorIs(t, CompleteTeamPayment(order.TradeNo, PaymentProviderBalance, "provider-a", "10.50", "alipay"), ErrPaymentMethodMismatch)
	assert.ErrorIs(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "0", "alipay"), ErrPaymentMethodMismatch)
	require.NoError(t, ExpireTeamPayment(order.TradeNo))
	assert.Error(t, CompleteTeamPayment(order.TradeNo, PaymentProviderEpay, "provider-a", "10.50", "alipay"),
		"late callbacks must not reactivate expired orders")
	var count int64
	require.NoError(t, DB.Model(&TeamSubscription{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPendingTeamPaymentCanBeResumedOnlyByItsOwnerWithoutNewOrder(t *testing.T) {
	team, owner, target := teamTestFixture(t)
	order := &TeamOrder{TeamId: team.Id, PayerUserId: owner.Id, PlanId: 12, PlanTitle: "Original shared plan",
		SeatLimit: 2, AmountTotal: 500, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		ResetPeriod: SubscriptionResetNever, Money: 10.50, TradeNo: "team-resume-" + t.Name(),
		PaymentMethod: "alipay", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, DB.Create(order).Error)
	got, err := GetPendingTeamPayment(owner.Id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, order.TradeNo, got.TradeNo)
	assert.Equal(t, order.Money, got.Money)
	assert.Equal(t, order.PlanTitle, got.PlanTitle)
	require.NoError(t, DB.Create(&TeamMember{TeamId: team.Id, UserId: target.Id, Role: "member"}).Error)
	other, err := GetPendingTeamPayment(target.Id)
	require.NoError(t, err)
	assert.Nil(t, other, "team members must not see or reopen the owner's payment")
	var count int64
	require.NoError(t, DB.Model(&TeamOrder{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, DB.Model(order).Update("status", common.TopUpStatusSuccess).Error)
	got, err = GetPendingTeamPayment(owner.Id)
	require.NoError(t, err)
	assert.Nil(t, got, "a completed payment must not be reopened")
}

func TestTeamCustomResetPeriodUsesOriginalTermBoundary(t *testing.T) {
	start := int64(1790500000)
	sub := &TeamSubscription{StartTime: start, EndTime: start + 600, ResetPeriod: SubscriptionResetCustom, ResetCustomSeconds: 60}
	periodStart, periodEnd := teamSubscriptionPeriodBounds(sub, start+125)
	assert.EqualValues(t, start+120, periodStart)
	assert.EqualValues(t, start+180, periodEnd)
	periodStart, periodEnd = teamSubscriptionPeriodBounds(sub, start+599)
	assert.EqualValues(t, start+540, periodStart)
	assert.EqualValues(t, start+600, periodEnd)
}

func TestTeamBalanceRenewalQueuesNewTermWithoutPersonalTopup(t *testing.T) {
	team, owner, _ := teamTestFixture(t)
	plan := &SubscriptionPlan{Scope: "team", SeatLimit: 2, Title: "Shared term", Enabled: true,
		PriceAmount: 1, TotalAmount: 800, DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		QuotaResetPeriod: SubscriptionResetNever}
	require.NoError(t, DB.Create(plan).Error)
	t.Cleanup(func() { _ = DB.Delete(plan).Error })
	require.NoError(t, DB.Model(owner).Update("quota", 2_000_000).Error)
	require.NoError(t, PurchaseTeamWithBalance(owner.Id, plan.Id))
	require.NoError(t, PurchaseTeamWithBalance(owner.Id, plan.Id))
	var subscriptions []TeamSubscription
	require.NoError(t, DB.Where("team_id = ?", team.Id).Order("start_time asc").Find(&subscriptions).Error)
	require.Len(t, subscriptions, 2)
	assert.Equal(t, subscriptions[0].EndTime, subscriptions[1].StartTime)
	assert.EqualValues(t, 800, subscriptions[1].AmountTotal)
	var updated User
	require.NoError(t, DB.First(&updated, owner.Id).Error)
	assert.EqualValues(t, 1_000_000, updated.Quota)
	var topups int64
	require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", owner.Id).Count(&topups).Error)
	assert.Zero(t, topups)
}
