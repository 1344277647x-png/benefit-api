package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	TeamStatusActive        = "active"
	TeamStatusSuspended     = "suspended"
	TeamInvitationPending   = "pending"
	TeamInvitationAccepted  = "accepted"
	TeamInvitationDeclined  = "declined"
	TeamInvitationCancelled = "cancelled"
)

type Team struct {
	Id        int    `json:"id"`
	OwnerId   int    `json:"owner_id" gorm:"uniqueIndex"`
	Name      string `json:"name" gorm:"type:varchar(80);not null"`
	Status    string `json:"status" gorm:"type:varchar(16);not null"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint"`
}

type TeamMember struct {
	Id       int    `json:"id"`
	TeamId   int    `json:"team_id" gorm:"index"`
	UserId   int    `json:"user_id" gorm:"uniqueIndex"`
	Role     string `json:"role" gorm:"type:varchar(16)"`
	JoinedAt int64  `json:"joined_at" gorm:"type:bigint"`
}

type TeamInvitation struct {
	Id           int    `json:"id"`
	TeamId       int    `json:"team_id" gorm:"index"`
	TargetUserId int    `json:"target_user_id" gorm:"index"`
	Email        string `json:"email" gorm:"type:varchar(255)"`
	Status       string `json:"status" gorm:"type:varchar(16);index"`
	ExpiresAt    int64  `json:"expires_at" gorm:"type:bigint"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint"`
}

// A paid term is independent of personal UserSubscription. Scheduled terms
// become usable only when start_time <= now, without a background activation job.
type TeamSubscription struct {
	Id                 int    `json:"id"`
	TeamId             int    `json:"team_id" gorm:"index"`
	PlanId             int    `json:"plan_id"`
	PlanTitle          string `json:"plan_title" gorm:"type:varchar(128)"`
	SeatLimit          int    `json:"seat_limit"`
	AmountTotal        int64  `json:"amount_total" gorm:"type:bigint"`
	ResetPeriod        string `json:"reset_period" gorm:"type:varchar(16)"`
	ResetCustomSeconds int64  `json:"reset_custom_seconds" gorm:"type:bigint"`
	StartTime          int64  `json:"start_time" gorm:"type:bigint"`
	EndTime            int64  `json:"end_time" gorm:"type:bigint;index"`
	Status             string `json:"status" gorm:"type:varchar(16);index"`
}

// Period rows keep outstanding requests pinned to their original quota window.
type TeamQuotaPeriod struct {
	Id                 int   `json:"id"`
	TeamSubscriptionId int   `json:"team_subscription_id" gorm:"uniqueIndex:idx_team_period"`
	StartTime          int64 `json:"start_time" gorm:"type:bigint;uniqueIndex:idx_team_period"`
	EndTime            int64 `json:"end_time" gorm:"type:bigint"`
	AmountTotal        int64 `json:"amount_total" gorm:"type:bigint"`
	AmountUsed         int64 `json:"amount_used" gorm:"type:bigint"`
}

// Snapshot purchased terms before handing control to the payment provider.
type TeamOrder struct {
	Id                 int             `json:"id"`
	TeamId             int             `json:"team_id" gorm:"index"`
	PayerUserId        int             `json:"payer_user_id"`
	PlanId             int             `json:"plan_id"`
	PlanTitle          string          `json:"plan_title"`
	SeatLimit          int             `json:"seat_limit"`
	AmountTotal        int64           `json:"amount_total"`
	DurationUnit       string          `json:"duration_unit"`
	DurationValue      int             `json:"duration_value"`
	CustomSeconds      int64           `json:"custom_seconds"`
	ResetPeriod        string          `json:"reset_period"`
	ResetCustomSeconds int64           `json:"reset_custom_seconds"`
	Money              TeamOrderAmount `json:"money"`
	TradeNo            string          `json:"trade_no" gorm:"type:varchar(128);uniqueIndex"`
	PaymentMethod      string          `json:"payment_method"`
	PaymentProvider    string          `json:"payment_provider"`
	ProviderTradeNo    *string         `json:"-" gorm:"type:varchar(128);uniqueIndex"`
	Status             string          `json:"status" gorm:"type:varchar(16);index"`
	CreateTime         int64           `json:"create_time"`
	CompleteTime       int64           `json:"complete_time"`
}

// SQLite's migration parser cannot rebuild a table containing DECIMAL(10,6)
// on the next startup. Use its compatible DECIMAL type there while retaining
// the bounded precision of the payment snapshot on MySQL and PostgreSQL.
type TeamOrderAmount float64

func (TeamOrderAmount) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "sqlite" {
		return "DECIMAL"
	}
	return "DECIMAL(10,6)"
}

type TeamUsage struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	TeamId             int    `json:"team_id" gorm:"index"`
	MemberUserId       int    `json:"member_user_id" gorm:"index"`
	TeamSubscriptionId int    `json:"team_subscription_id" gorm:"index"`
	PeriodId           int    `json:"period_id" gorm:"index"`
	Reserved           int64  `json:"reserved" gorm:"type:bigint"`
	FinalAmount        int64  `json:"final_amount" gorm:"type:bigint"`
	// Persist the actual upstream cost before synchronous settlement. An
	// under-reserved request must not be mistaken for an abandoned reservation.
	PendingFinalAmount int64 `json:"-" gorm:"type:bigint"`
	PendingFinal       bool  `json:"-"`
	// Marks the last durable point before sending the request upstream. If a
	// process crashes without a final intent, the outcome cannot be inferred
	// from the reservation alone and must not be silently refunded.
	AttemptedAt int64  `json:"-" gorm:"type:bigint"`
	ResolvedBy  int    `json:"-"`
	ResolvedAt  int64  `json:"-" gorm:"type:bigint"`
	EvidenceRef string `json:"-" gorm:"type:varchar(128)"`
	Status      string `json:"status" gorm:"type:varchar(16)"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;index"`
}

// TeamOwnRequestStatus is an audit view for the member who made the request.
// It intentionally contains no prompt, media, key or other member's usage.
type TeamOwnRequestStatus struct {
	RequestId     string `json:"request_id"`
	Status        string `json:"status"`
	Reserved      int64  `json:"reserved"`
	FinalAmount   int64  `json:"final_amount"`
	PendingAmount int64  `json:"pending_amount,omitempty"`
	ResponseState string `json:"response_state"`
	UnbilledQuota int64  `json:"unbilled_quota,omitempty"`
	CreditedQuota int64  `json:"credited_quota,omitempty"`
}

// TeamRequestAudit is a limited accounting view for manual reconciliation.
// It deliberately excludes request bodies, upstream responses and secrets.
type TeamRequestAudit struct {
	TeamId        int    `json:"team_id"`
	MemberUserId  int    `json:"member_user_id"`
	RequestId     string `json:"request_id"`
	Status        string `json:"status"`
	Reserved      int64  `json:"reserved"`
	FinalAmount   int64  `json:"final_amount"`
	PendingAmount int64  `json:"pending_amount,omitempty"`
	Shortfall     int64  `json:"shortfall,omitempty"`
	PeriodEndTime int64  `json:"period_end_time,omitempty"`
	AttemptedAt   int64  `json:"attempted_at"`
	CreatedAt     int64  `json:"created_at"`
	ResponseState string `json:"response_state"`
}

func PendingTeamRequestAudits(limit int) ([]TeamRequestAudit, error) {
	if limit <= 0 || limit > 100 {
		return nil, errors.New("invalid team audit limit")
	}
	var usages []TeamUsage
	if err := DB.Where("status = ? AND (pending_final = ? OR attempted_at > 0)", "reserved", true).
		Order("created_at asc, id asc").Limit(limit).Find(&usages).Error; err != nil {
		return nil, err
	}
	audits := make([]TeamRequestAudit, 0, len(usages))
	for _, usage := range usages {
		status := "upstream_outcome_unconfirmed"
		response := "not_recorded"
		var shortfall, periodEndTime int64
		if usage.PendingFinal {
			status = "pending_settlement"
			response = "withheld_pending_settlement"
			var period TeamQuotaPeriod
			if err := DB.Select("amount_total", "amount_used", "end_time").First(&period, usage.PeriodId).Error; err != nil {
				return nil, err
			}
			periodEndTime = period.EndTime
			needed := usage.PendingFinalAmount - usage.Reserved
			if needed > 0 {
				available := period.AmountTotal - period.AmountUsed
				if available < needed {
					shortfall = needed - max(available, 0)
				}
			}
		}
		audits = append(audits, TeamRequestAudit{
			TeamId: usage.TeamId, MemberUserId: usage.MemberUserId,
			RequestId: usage.RequestId, Status: status, Reserved: usage.Reserved,
			FinalAmount: usage.FinalAmount, AttemptedAt: usage.AttemptedAt,
			PendingAmount: usage.PendingFinalAmount, CreatedAt: usage.CreatedAt,
			Shortfall: shortfall, PeriodEndTime: periodEndTime,
			ResponseState: response,
		})
	}
	return audits, nil
}

func GetTeamOwnRequestStatus(requestId string, teamId, userId int) (*TeamOwnRequestStatus, error) {
	if teamId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return getTeamRequestStatus(requestId, teamId, userId)
}

func getTeamRequestStatus(requestId string, teamId, userId int) (*TeamOwnRequestStatus, error) {
	if len(requestId) == 0 || len(requestId) > 64 || userId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var usage TeamUsage
	query := DB.Where("request_id = ? AND member_user_id = ?", requestId, userId)
	if teamId > 0 {
		query = query.Where("team_id = ?", teamId)
	}
	if err := query.First(&usage).Error; err != nil {
		return nil, err
	}
	result := &TeamOwnRequestStatus{RequestId: requestId, Status: usage.Status,
		Reserved: usage.Reserved, FinalAmount: usage.FinalAmount, ResponseState: "not_recorded"}
	if usage.Status == "reserved" && usage.AttemptedAt > 0 {
		result.Status = "upstream_outcome_unconfirmed"
	}
	if usage.PendingFinal {
		result.Status = "pending_settlement"
		result.PendingAmount = usage.PendingFinalAmount
		result.ResponseState = "withheld_pending_settlement"
	}
	var event TeamSyncBillingEvent
	if err := DB.Where("request_id = ?", requestId).First(&event).Error; err == nil {
		if usage.PendingFinal {
			return result, nil
		}
		result.ResponseState = event.ResponseState
		result.CreditedQuota = event.CreditedQuota
		if event.ResponseState == "compensated_no_delivery" {
			result.UnbilledQuota = event.FinalQuota
		}
		if result.ResponseState == "" {
			result.ResponseState = "unconfirmed"
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return result, nil
}

// A member keeps access to the accounting status of their own past request
// after leaving the team. Request IDs alone never grant access to another
// member's result or to a team's private details.
func GetTeamMemberRequestStatus(requestId string, userId int) (*TeamOwnRequestStatus, error) {
	return getTeamRequestStatus(requestId, 0, userId)
}

// Recent requests remain discoverable after a member leaves a team. A client
// that disconnects before receiving the request-ID header can still find its
// own unsettled or charged request without learning another member's data.
func ListTeamMemberRequestStatuses(userId, limit int, beforeRequestId string) ([]TeamOwnRequestStatus, error) {
	if userId <= 0 || limit <= 0 || limit > 100 {
		return nil, errors.New("invalid team request status query")
	}
	if len(beforeRequestId) > 64 {
		return nil, errors.New("invalid team request cursor")
	}
	query := DB.Select("request_id").Where("member_user_id = ?", userId)
	if beforeRequestId != "" {
		var cursor TeamUsage
		if err := DB.Select("id").Where("request_id = ? AND member_user_id = ?", beforeRequestId, userId).
			First(&cursor).Error; err != nil {
			return nil, err
		}
		query = query.Where("id < ?", cursor.Id)
	}
	var usages []TeamUsage
	if err := query.Order("id desc").Limit(limit).Find(&usages).Error; err != nil {
		return nil, err
	}
	statuses := make([]TeamOwnRequestStatus, 0, len(usages))
	for _, usage := range usages {
		status, err := getTeamRequestStatus(usage.RequestId, 0, userId)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, *status)
	}
	return statuses, nil
}

// MarkTeamSyncUpstreamAttempt persists the ambiguity before any upstream
// bytes are sent. A second invocation with the same ID cannot start another
// upstream operation; retries within one Relay call reuse its first marker.
func MarkTeamSyncUpstreamAttempt(requestId string) error {
	if requestId == "" || len(requestId) > 64 {
		return errors.New("invalid team request id")
	}
	result := DB.Model(&TeamUsage{}).
		Where("request_id = ? AND status = ? AND pending_final = ? AND attempted_at = 0", requestId, "reserved", false).
		Update("attempted_at", GetDBTimestamp())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	return errors.New("team upstream attempt conflicts with request state")
}

func TeamUpstreamAttempted(requestId string) (bool, error) {
	if requestId == "" || len(requestId) > 64 {
		return false, errors.New("invalid team request id")
	}
	var usage TeamUsage
	if err := DB.Select("attempted_at").Where("request_id = ?", requestId).First(&usage).Error; err != nil {
		return false, err
	}
	return usage.AttemptedAt > 0, nil
}

// The settlement and counters commit in the main database; the independent
// log database consumes this event with a unique receipt after commit.
type TeamSyncBillingEvent struct {
	RequestId        string `gorm:"primaryKey;type:varchar(64)"`
	TeamId           int
	UserId           int
	ChannelId        int
	TokenId          int
	Group            string `gorm:"type:varchar(50)"`
	ModelName        string `gorm:"type:varchar(191)"`
	PromptTokens     int
	CompletionTokens int
	UseTimeSeconds   int
	IsStream         bool
	BillingDetails   string `gorm:"type:text"`
	// Server write completion is not a client receipt. A crash or broken
	// connection leaves this explicitly unconfirmed for manual reconciliation.
	ResponseState string `gorm:"type:varchar(32)"`
	FinalQuota    int64  `gorm:"type:bigint"`
	CreatedAt     int64  `gorm:"type:bigint"`
	Ready         bool   `gorm:"index"`
	DeliveredAt   int64  `gorm:"type:bigint;index"`
	// An operator-approved delivery-dispute credit is separate from the
	// upstream consumption. The original gross usage/log remain unchanged;
	// only the original team's quota period receives this one-time credit.
	CreditedQuota int64 `gorm:"type:bigint"`
	CreditBy      int
	CreditAt      int64  `gorm:"type:bigint"`
	CreditRef     string `gorm:"type:varchar(128)"`
}

func MarkTeamSyncResponseState(requestId, state string) error {
	if requestId == "" || (state != "withheld" && state != "writing" && state != "server_write_completed" && state != "write_failed") {
		return errors.New("invalid team response state")
	}
	allowed := map[string][]string{
		"withheld":               {""},
		"writing":                {""},
		"server_write_completed": {"writing"},
		"write_failed":           {"writing"},
	}
	result := DB.Model(&TeamSyncBillingEvent{}).
		Where("request_id = ? AND ready = ? AND response_state IN ?", requestId, true, allowed[state]).
		Update("response_state", state)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("team response state transition is invalid")
	}
	return nil
}

type TeamSyncLogReceipt struct {
	RequestId string `gorm:"primaryKey;type:varchar(64)"`
	CreatedAt int64  `gorm:"type:bigint"`
}

type TeamSyncBillingMeta struct {
	ChannelId        int
	TokenId          int
	Group            string
	ModelName        string
	PromptTokens     int
	CompletionTokens int
	UseTimeSeconds   int
	IsStream         bool
	BillingDetails   string
}

// Committed with the terminal task, so a crash between funding settlement and
// the log database write cannot erase the audit adjustment.
type TeamTaskBillingEvent struct {
	TaskId        int64  `gorm:"primaryKey;autoIncrement:false"`
	PublicTaskId  string `gorm:"type:varchar(191)"`
	TeamId        int
	UserId        int
	ChannelId     int
	TokenId       int
	Group         string `gorm:"type:varchar(50)"`
	ModelName     string `gorm:"type:varchar(191)"`
	PreviousQuota int64  `gorm:"type:bigint"`
	FinalQuota    int64  `gorm:"type:bigint"`
	Reason        string `gorm:"type:text"`
	ClampJSON     string `gorm:"type:text"`
	OtherJSON     string `gorm:"type:text"`
	CreatedAt     int64  `gorm:"type:bigint"`
	DeliveredAt   int64  `gorm:"type:bigint;index"`
}

// Lives in LOG_DB and is committed in the *same log transaction* as the log.
// A repeated delivery after the log commit is a no-op, even if the primary
// database's delivered marker was not updated before the process crashed.
type TeamTaskLogReceipt struct {
	TaskId    int64 `gorm:"primaryKey;autoIncrement:false"`
	CreatedAt int64 `gorm:"type:bigint"`
}

func GetMyTeam(userId int) (*Team, *TeamMember, error) {
	var member TeamMember
	if err := DB.Where("user_id = ?", userId).First(&member).Error; err != nil {
		return nil, nil, err
	}
	var team Team
	if err := DB.First(&team, member.TeamId).Error; err != nil {
		return nil, nil, err
	}
	return &team, &member, nil
}

func CreateTeam(userId int, name string) (*Team, error) {
	name = strings.TrimSpace(name)
	if userId <= 0 || name == "" || len([]rune(name)) > 80 {
		return nil, errors.New("invalid team name or owner")
	}
	team := &Team{OwnerId: userId, Name: name, Status: TeamStatusActive, CreatedAt: GetDBTimestamp()}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var owner User
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", userId, common.UserStatusEnabled).First(&owner).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&TeamMember{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("account already belongs to a team")
		}
		if err := tx.Create(team).Error; err != nil {
			return err
		}
		return tx.Create(&TeamMember{TeamId: team.Id, UserId: userId, Role: "owner", JoinedAt: team.CreatedAt}).Error
	})
	return team, err
}

func teamSeatCapTx(tx *gorm.DB, teamId int, now int64) (int, error) {
	var subscriptions []TeamSubscription
	if err := tx.Where("team_id = ? AND status = ? AND end_time > ?", teamId, TeamStatusActive, now).Find(&subscriptions).Error; err != nil {
		return 0, err
	}
	cap := 0
	active := false
	for _, sub := range subscriptions {
		if sub.StartTime <= now {
			active = true
		}
		if cap == 0 || sub.SeatLimit < cap {
			cap = sub.SeatLimit
		}
	}
	if !active {
		return 0, nil
	}
	var orders []TeamOrder
	// A signed callback can arrive after a browser session or a 30-minute
	// window. Pending payments must constrain seats until explicitly resolved.
	if err := tx.Where("team_id = ? AND status = ?", teamId, common.TopUpStatusPending).Find(&orders).Error; err != nil {
		return 0, err
	}
	for _, order := range orders {
		if cap == 0 || order.SeatLimit < cap {
			cap = order.SeatLimit
		}
	}
	return cap, nil
}

func InviteTeamMember(ownerId int, email string) (*TeamInvitation, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, errors.New("email is required")
	}
	var targets []User
	if err := DB.Where("LOWER(email) = ? AND status = ?", email, common.UserStatusEnabled).Limit(2).Find(&targets).Error; err != nil {
		return nil, err
	}
	if len(targets) != 1 || targets[0].Id == ownerId {
		return nil, errors.New("no unique eligible account for this email")
	}
	now := GetDBTimestamp()
	var invite *TeamInvitation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("owner_id = ? AND status = ?", ownerId, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		cap, err := teamSeatCapTx(tx, team.Id, now)
		if err != nil {
			return err
		}
		if cap == 0 {
			return errors.New("active team subscription required")
		}
		var count int64
		if err := tx.Model(&TeamMember{}).Where("user_id = ?", targets[0].Id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("account already belongs to a team")
		}
		if err := tx.Model(&TeamInvitation{}).Where("team_id = ? AND target_user_id = ? AND status = ? AND expires_at > ?", team.Id, targets[0].Id, TeamInvitationPending, now).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("invitation already pending")
		}
		if err := tx.Model(&TeamMember{}).Where("team_id = ?", team.Id).Count(&count).Error; err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&TeamInvitation{}).Where("team_id = ? AND status = ? AND expires_at > ?", team.Id, TeamInvitationPending, now).Count(&pending).Error; err != nil {
			return err
		}
		if count+pending >= int64(cap) {
			return errors.New("team seats full")
		}
		invite = &TeamInvitation{TeamId: team.Id, TargetUserId: targets[0].Id, Email: email, Status: TeamInvitationPending, ExpiresAt: now + 7*24*3600, CreatedAt: now}
		return tx.Create(invite).Error
	})
	return invite, err
}

func AcceptTeamInvitation(userId, invitationId int) error {
	now := GetDBTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var invite TeamInvitation
		if err := lockForUpdate(tx).Where("id = ? AND target_user_id = ?", invitationId, userId).First(&invite).Error; err != nil {
			return err
		}
		if invite.Status != TeamInvitationPending || invite.ExpiresAt <= now {
			return errors.New("invitation is not pending")
		}
		var target User
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", userId, common.UserStatusEnabled).First(&target).Error; err != nil {
			return err
		}
		var team Team
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", invite.TeamId, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		cap, err := teamSeatCapTx(tx, team.Id, now)
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&TeamMember{}).Where("team_id = ?", team.Id).Count(&count).Error; err != nil {
			return err
		}
		if cap == 0 || count >= int64(cap) {
			return errors.New("team seats full or subscription expired")
		}
		if err := tx.Create(&TeamMember{TeamId: team.Id, UserId: userId, Role: "member", JoinedAt: now}).Error; err != nil {
			return err
		}
		return tx.Model(&invite).Update("status", TeamInvitationAccepted).Error
	})
}

func RemoveTeamMember(actorId, targetId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		var actor TeamMember
		if err := tx.Where("user_id = ?", actorId).First(&actor).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&team, actor.TeamId).Error; err != nil {
			return err
		}
		if targetId == team.OwnerId || (actorId != targetId && actorId != team.OwnerId) {
			return errors.New("not allowed to remove this member")
		}
		result := tx.Where("team_id = ? AND user_id = ?", team.Id, targetId).Delete(&TeamMember{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func RespondToTeamInvitation(userId, invitationId int, accept bool) error {
	if accept {
		return AcceptTeamInvitation(userId, invitationId)
	}
	result := DB.Model(&TeamInvitation{}).Where("id = ? AND target_user_id = ? AND status = ? AND expires_at > ?",
		invitationId, userId, TeamInvitationPending, common.GetTimestamp()).
		Update("status", TeamInvitationDeclined)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func CancelTeamInvitation(ownerId, invitationId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("owner_id = ?", ownerId).First(&team).Error; err != nil {
			return err
		}
		result := tx.Model(&TeamInvitation{}).Where("id = ? AND team_id = ? AND status = ?", invitationId, team.Id, TeamInvitationPending).
			Update("status", TeamInvitationCancelled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func TeamMemberUsage(teamId int) ([]struct {
	UserId   int   `json:"user_id"`
	Amount   int64 `json:"amount"`
	Requests int64 `json:"requests"`
}, error) {
	var rows []struct {
		UserId   int   `json:"user_id"`
		Amount   int64 `json:"amount"`
		Requests int64 `json:"requests"`
	}
	err := DB.Model(&TeamUsage{}).Select("member_user_id AS user_id, COALESCE(SUM(final_amount),0) AS amount, COUNT(*) AS requests").
		Where("team_id = ? AND status = ?", teamId, "settled").Group("member_user_id").Scan(&rows).Error
	return rows, err
}

// Member-facing usage is scoped to the current team and account. It does not
// read mutable request logs, prompts, token secrets or other members' usage.
func TeamOwnUsage(teamId, userId int) (amount, requests int64, err error) {
	var summary struct {
		Amount   int64
		Requests int64
	}
	err = DB.Model(&TeamUsage{}).
		Select("COALESCE(SUM(final_amount),0) AS amount, COUNT(*) AS requests").
		Where("team_id = ? AND member_user_id = ? AND status = ?", teamId, userId, "settled").
		Scan(&summary).Error
	return summary.Amount, summary.Requests, err
}

// Gross member consumption may exceed net period usage after a documented
// delivery-dispute credit. Expose the aggregate only, never a member's log.
func TeamPeriodCreditedQuota(periodId int) (int64, error) {
	if periodId <= 0 {
		return 0, errors.New("invalid team quota period")
	}
	var result struct{ Total int64 }
	err := DB.Model(&TeamSyncBillingEvent{}).
		Select("COALESCE(SUM(team_sync_billing_events.credited_quota),0) AS total").
		Joins("JOIN team_usages ON team_usages.request_id = team_sync_billing_events.request_id").
		Where("team_usages.period_id = ?", periodId).Scan(&result).Error
	return result.Total, err
}

func teamSubscriptionPeriodBounds(sub *TeamSubscription, now int64) (int64, int64) {
	start := sub.StartTime
	end := sub.EndTime
	if sub.ResetPeriod == SubscriptionResetNever {
		return start, end
	}
	if sub.ResetPeriod == SubscriptionResetCustom && sub.ResetCustomSeconds > 0 {
		interval := sub.ResetCustomSeconds
		if now > start {
			start += ((now - start) / interval) * interval
		}
		if end-start > interval {
			end = start + interval
		}
		return start, end
	}
	plan := &SubscriptionPlan{QuotaResetPeriod: sub.ResetPeriod, QuotaResetCustomSeconds: sub.ResetCustomSeconds}
	for next := calcNextResetTime(time.Unix(start, 0), plan, end); next > start && next <= now && next < end; next = calcNextResetTime(time.Unix(start, 0), plan, end) {
		start = next
	}
	if next := calcNextResetTime(time.Unix(start, 0), plan, end); next > start && next < end {
		end = next
	}
	return start, end
}

func currentTeamPeriodTx(tx *gorm.DB, sub *TeamSubscription, now int64) (*TeamQuotaPeriod, error) {
	start, end := teamSubscriptionPeriodBounds(sub, now)
	var period TeamQuotaPeriod
	result := tx.Where("team_subscription_id = ? AND start_time = ?", sub.Id, start).First(&period)
	if result.Error == nil {
		return &period, nil
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	period = TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: start, EndTime: end, AmountTotal: sub.AmountTotal}
	if err := tx.Create(&period).Error; err != nil {
		return nil, err
	}
	return &period, nil
}

// PreConsumeTeamForToken rechecks the live credential in the same transaction
// as the reservation. TokenAuth can precede channel selection by several
// operations, during which the owner may revoke the token or membership.
func PreConsumeTeamForToken(requestId string, teamId, userId, tokenId int, amount int64) (*TeamUsage, *TeamQuotaPeriod, error) {
	if tokenId <= 0 {
		return nil, nil, errors.New("team token is required")
	}
	return reserveTeamQuota(requestId, teamId, userId, tokenId, amount)
}

func reserveTeamQuota(requestId string, teamId, userId, tokenId int, amount int64) (*TeamUsage, *TeamQuotaPeriod, error) {
	if requestId == "" || teamId <= 0 || userId <= 0 || amount <= 0 {
		return nil, nil, errors.New("invalid team reservation")
	}
	now := GetDBTimestamp()
	var usage TeamUsage
	var period TeamQuotaPeriod
	err := DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", teamId, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		if tokenId > 0 {
			var token Token
			if err := lockForUpdate(tx).Where("id = ? AND team_id = ? AND user_id = ? AND team_enabled = ? AND status = ?",
				tokenId, teamId, userId, true, common.TokenStatusDisabled).First(&token).Error; err != nil {
				return ErrTokenInvalid
			}
			if !strings.HasPrefix(token.Key, teamTokenPrefix) ||
				(token.ExpiredTime != -1 && token.ExpiredTime <= now) {
				return ErrTokenInvalid
			}
		}
		var member TeamMember
		if err := tx.Where("team_id = ? AND user_id = ?", teamId, userId).First(&member).Error; err != nil {
			return err
		}
		var existing TeamUsage
		result := tx.Where("request_id = ?", requestId).First(&existing)
		if result.Error == nil {
			if tokenId > 0 {
				// Reusing a live request ID could make a second handler refund the
				// first handler's reservation on its own failure.
				return errors.New("team request id already reserved")
			}
			if existing.TeamId != teamId || existing.MemberUserId != userId || existing.Status != "reserved" || existing.Reserved != amount {
				return errors.New("request already used for another team or refunded")
			}
			usage = existing
			return tx.First(&period, existing.PeriodId).Error
		}
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return result.Error
		}
		var sub TeamSubscription
		if err := lockForUpdate(tx).Where("team_id = ? AND status = ? AND start_time <= ? AND end_time > ?", teamId, TeamStatusActive, now, now).Order("start_time desc").First(&sub).Error; err != nil {
			return errors.New("no active team subscription")
		}
		p, err := currentTeamPeriodTx(tx, &sub, now)
		if err != nil {
			return err
		}
		period = *p
		if amount > period.AmountTotal || period.AmountUsed > period.AmountTotal-amount {
			return errors.New("team subscription quota insufficient")
		}
		updated := tx.Model(&TeamQuotaPeriod{}).Where("id = ? AND amount_used <= ?", period.Id, period.AmountTotal-amount).
			Update("amount_used", gorm.Expr("amount_used + ?", amount))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("team subscription quota insufficient")
		}
		period.AmountUsed += amount
		usage = TeamUsage{RequestId: requestId, TeamId: teamId, MemberUserId: userId, TeamSubscriptionId: sub.Id, PeriodId: period.Id, Reserved: amount, Status: "reserved", CreatedAt: now}
		return tx.Create(&usage).Error
	})
	return &usage, &period, err
}

// RecordTeamSyncSettlementIntent makes an upstream-successful request
// distinguishable from an abandoned reservation after a process crash. It
// never charges a different period, team or the member's personal wallet.
func RecordTeamSyncSettlementIntent(requestId string, finalAmount int64, metadata ...TeamSyncBillingMeta) error {
	if requestId == "" || finalAmount < 0 || finalAmount > common.MaxQuota {
		return errors.New("invalid team settlement intent")
	}
	createdAt := GetDBTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.Status == "settled" && usage.FinalAmount == finalAmount {
			return nil
		}
		if usage.Status != "reserved" || (usage.PendingFinal && usage.PendingFinalAmount != finalAmount) {
			return errors.New("team settlement intent conflicts with request state")
		}
		if usage.PendingFinal {
			var event TeamSyncBillingEvent
			if err := tx.Where("request_id = ?", requestId).First(&event).Error; err != nil || event.FinalQuota != finalAmount ||
				event.UserId != usage.MemberUserId || event.TeamId != usage.TeamId {
				return errors.New("team settlement intent is missing or inconsistent")
			}
			return nil
		}
		if err := tx.Model(&usage).Updates(map[string]any{
			"pending_final": true, "pending_final_amount": finalAmount,
		}).Error; err != nil {
			return err
		}
		meta := TeamSyncBillingMeta{}
		if len(metadata) > 0 {
			meta = metadata[0]
		}
		if meta.PromptTokens < 0 || meta.CompletionTokens < 0 || meta.UseTimeSeconds < 0 || meta.ChannelId < 0 || meta.TokenId < 0 {
			return errors.New("invalid team billing metadata")
		}
		if len(meta.BillingDetails) > 16384 {
			return errors.New("team billing details exceed size limit")
		}
		return tx.Create(&TeamSyncBillingEvent{RequestId: requestId, TeamId: usage.TeamId,
			UserId: usage.MemberUserId, ChannelId: meta.ChannelId, TokenId: meta.TokenId,
			Group: meta.Group, ModelName: meta.ModelName, PromptTokens: meta.PromptTokens,
			CompletionTokens: meta.CompletionTokens, UseTimeSeconds: meta.UseTimeSeconds,
			IsStream: meta.IsStream, BillingDetails: meta.BillingDetails, FinalQuota: finalAmount,
			CreatedAt: createdAt}).Error
	})
}

// AdjustTeamUsage records reservation increments or final settlement atomically.
func AdjustTeamUsage(requestId string, delta int64, final bool) error {
	return adjustTeamUsage(requestId, delta, final, nil)
}

// Final settlement uses an absolute amount: a recovery worker can commit
// between recording the intent and the original handler settling it.
func SettleTeamSyncUsage(requestId string, finalAmount int64) error {
	if finalAmount < 0 || finalAmount > common.MaxQuota {
		return errors.New("invalid team final amount")
	}
	return adjustTeamUsage(requestId, 0, true, &finalAmount)
}

func adjustTeamUsage(requestId string, delta int64, final bool, finalAmount *int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.Status == "settled" {
			if finalAmount != nil {
				if usage.FinalAmount == *finalAmount {
					return nil
				}
				return errors.New("team final amount conflicts with settlement")
			}
			if final && delta == 0 {
				return nil
			}
			return errors.New("team request already settled")
		}
		if usage.Status != "reserved" {
			return errors.New("team request already refunded")
		}
		if finalAmount != nil {
			if !usage.PendingFinal || usage.PendingFinalAmount != *finalAmount {
				return errors.New("team final amount conflicts with persisted intent")
			}
			delta = *finalAmount - usage.Reserved
		}
		if final && usage.PendingFinal &&
			(delta > common.MaxQuota || delta < -common.MaxQuota || usage.Reserved+delta != usage.PendingFinalAmount) {
			return errors.New("team settlement does not match persisted intent")
		}
		if !final && usage.PendingFinal {
			return errors.New("team request is pending final settlement")
		}
		hadSettlementIntent := usage.PendingFinal
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		if delta > 0 && (delta > period.AmountTotal || period.AmountUsed > period.AmountTotal-delta) {
			return errors.New("team subscription quota insufficient")
		}
		if delta < 0 && (usage.Reserved < -delta || period.AmountUsed < -delta) {
			return errors.New("invalid team refund amount")
		}
		period.AmountUsed += delta
		if err := tx.Save(&period).Error; err != nil {
			return err
		}
		usage.Reserved += delta
		if final {
			usage.Status = "settled"
			usage.FinalAmount = usage.Reserved
			usage.PendingFinal = false
		}
		if err := tx.Save(&usage).Error; err != nil {
			return err
		}
		if final && hadSettlementIntent {
			var event TeamSyncBillingEvent
			result := tx.Where("request_id = ?", requestId).First(&event)
			if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			if result.Error == nil {
				if event.FinalQuota != usage.FinalAmount || event.UserId != usage.MemberUserId || event.TeamId != usage.TeamId {
					return errors.New("team billing event does not match settlement")
				}
				if updated := tx.Model(&User{}).Where("id = ?", event.UserId).
					Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", event.FinalQuota),
						"request_count": gorm.Expr("request_count + 1")}); updated.Error != nil || updated.RowsAffected != 1 {
					return errors.New("team sync user counter update failed")
				}
				if event.ChannelId > 0 {
					if updated := tx.Model(&Channel{}).Where("id = ?", event.ChannelId).
						Update("used_quota", gorm.Expr("used_quota + ?", event.FinalQuota)); updated.Error != nil || updated.RowsAffected != 1 {
						return errors.New("team sync channel counter update failed")
					}
				}
				return tx.Model(&event).Update("ready", true).Error
			} else {
				return errors.New("team settlement event is missing")
			}
		}
		return nil
	})
}

// RecoverPendingTeamSettlements commits funding, counters and a log outbox in
// one main-database transaction. Independent log delivery uses a receipt.
func RecoverPendingTeamSettlements(limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		return 0, errors.New("invalid pending team settlement limit")
	}
	var usages []TeamUsage
	// Long-lived underfunded requests must not occupy every recovery batch
	// and prevent other teams' affordable requests from ever being settled.
	// This is only candidate selection; the settlement transaction rechecks
	// the original period under a lock before changing any accounting data.
	if err := DB.Model(&TeamUsage{}).Select("team_usages.*").
		Joins("JOIN team_quota_periods ON team_quota_periods.id = team_usages.period_id").
		Where("team_usages.status = ? AND team_usages.pending_final = ?", "reserved", true).
		Where("team_usages.pending_final_amount <= team_usages.reserved OR team_quota_periods.amount_total - team_quota_periods.amount_used >= team_usages.pending_final_amount - team_usages.reserved").
		Order("team_usages.id asc").Limit(limit).Find(&usages).Error; err != nil {
		return 0, err
	}
	recovered := 0
	for _, usage := range usages {
		if err := SettleTeamSyncUsage(usage.RequestId, usage.PendingFinalAmount); err != nil {
			if strings.Contains(err.Error(), "quota insufficient") {
				continue
			}
			// Another worker may have committed the same persisted amount after
			// this batch was read. Only that exact terminal state is a no-op.
			var current TeamUsage
			if loadErr := DB.Where("request_id = ?", usage.RequestId).First(&current).Error; loadErr == nil &&
				current.Status == "settled" && current.FinalAmount == usage.PendingFinalAmount {
				continue
			}
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func RefundTeamUsage(requestId string) error {
	_, err := RefundTeamUsageOnce(requestId)
	return err
}

// RefundTeamUsageOnce reports whether this invocation actually released the
// reservation, so callers do not decrement usage statistics twice on retry.
func RefundTeamUsageOnce(requestId string) (bool, error) {
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.Status == "refunded" {
			return nil
		}
		if usage.Status != "reserved" {
			return errors.New("settled team request cannot be refunded")
		}
		if usage.PendingFinal {
			return errors.New("team request has an unresolved upstream settlement")
		}
		if usage.AttemptedAt > 0 {
			return errors.New("team upstream outcome requires documented reconciliation")
		}
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		if period.AmountUsed < usage.Reserved {
			return errors.New("invalid team period balance")
		}
		period.AmountUsed -= usage.Reserved
		if err := tx.Save(&period).Error; err != nil {
			return err
		}
		usage.Status = "refunded"
		if err := tx.Save(&usage).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied && err == nil, err
}

// ResolveTeamAttemptAsFailed releases an ambiguous reservation only after an
// administrator verified the provider did not deliver a billable result. The
// evidence reference is a ticket or provider transaction ID, never raw
// response content. The original period receives the credit exactly once.
func ResolveTeamAttemptAsFailed(requestId string, reviewerId int, evidenceRef string) (bool, error) {
	if requestId == "" || len(requestId) > 64 || reviewerId <= 0 || !validTeamEvidenceRef(evidenceRef) {
		return false, errors.New("invalid team reconciliation evidence")
	}
	applied := false
	resolvedAt := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.Status == "refunded" && usage.ResolvedBy == reviewerId && usage.EvidenceRef == evidenceRef {
			return nil
		}
		if usage.Status != "reserved" || usage.AttemptedAt == 0 || usage.PendingFinal {
			return errors.New("team request is not eligible for failed outcome reconciliation")
		}
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		if period.AmountUsed < usage.Reserved {
			return errors.New("invalid team period balance")
		}
		if err := tx.Model(&period).Update("amount_used", gorm.Expr("amount_used - ?", usage.Reserved)).Error; err != nil {
			return err
		}
		if err := tx.Model(&usage).Updates(map[string]any{
			"status": "refunded", "resolved_by": reviewerId,
			"resolved_at": resolvedAt, "evidence_ref": evidenceRef,
		}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied && err == nil, err
}

func validTeamEvidenceRef(evidenceRef string) bool {
	if len(evidenceRef) < 8 || len(evidenceRef) > 128 {
		return false
	}
	for _, r := range evidenceRef {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

// ResolveTeamAttemptAsBilled is only for an administrator holding provider
// evidence for the exact charge. It never invents a response, token usage or
// a pricing breakdown. If this term lacks the remaining quota, the persisted
// intent stays pending until funds in the original period become available.
func ResolveTeamAttemptAsBilled(requestId string, reviewerId int, evidenceRef string, finalAmount int64) (bool, error) {
	if requestId == "" || len(requestId) > 64 || reviewerId <= 0 || !validTeamEvidenceRef(evidenceRef) ||
		finalAmount < 0 || finalAmount > common.MaxQuota {
		return false, errors.New("invalid team reconciliation evidence or amount")
	}
	applied := false
	resolvedAt := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.ResolvedBy == reviewerId && usage.EvidenceRef == evidenceRef && usage.PendingFinalAmount == finalAmount &&
			(usage.PendingFinal || usage.Status == "settled" && usage.FinalAmount == finalAmount) {
			return nil
		}
		if usage.Status != "reserved" || usage.AttemptedAt == 0 || usage.PendingFinal || usage.ResolvedBy != 0 {
			return errors.New("team request is not eligible for billed outcome reconciliation")
		}
		if err := tx.Model(&usage).Updates(map[string]any{
			"pending_final": true, "pending_final_amount": finalAmount,
			"resolved_by": reviewerId, "resolved_at": resolvedAt, "evidence_ref": evidenceRef,
		}).Error; err != nil {
			return err
		}
		details, err := common.Marshal(map[string]any{
			"manual_reconciliation": "provider_confirmed_billed",
			"pricing_breakdown":     "not_recorded",
			"admin_info":            map[string]any{"evidence_ref": evidenceRef, "reviewer_id": reviewerId},
		})
		if err != nil {
			return err
		}
		if err := tx.Create(&TeamSyncBillingEvent{RequestId: requestId, TeamId: usage.TeamId,
			UserId: usage.MemberUserId, FinalQuota: finalAmount, BillingDetails: string(details),
			ResponseState: "withheld", CreatedAt: resolvedAt}).Error; err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := SettleTeamSyncUsage(requestId, finalAmount); err != nil && !strings.Contains(err.Error(), "quota insufficient") {
		return applied, err
	}
	return applied, nil
}

// CompensateTeamUndeliveredCharge is an explicit operator loss decision for
// a provider-billed response that could not be delivered or fully charged to
// its original period. It releases the member's reservation and retains the
// calculated user-side quota for audit, not the provider's actual invoice.
// It never sends that response or charges a wallet.
func CompensateTeamUndeliveredCharge(requestId string, reviewerId int, evidenceRef string) (bool, int64, error) {
	if requestId == "" || len(requestId) > 64 || reviewerId <= 0 || !validTeamEvidenceRef(evidenceRef) {
		return false, 0, errors.New("invalid team compensation evidence")
	}
	applied := false
	var unbilledQuota int64
	resolvedAt := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		var event TeamSyncBillingEvent
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&event).Error; err != nil {
			return err
		}
		if usage.Status == "refunded" && !usage.PendingFinal &&
			usage.ResolvedBy == reviewerId && usage.EvidenceRef == evidenceRef &&
			event.ResponseState == "compensated_no_delivery" && !event.Ready && event.DeliveredAt == 0 {
			unbilledQuota = event.FinalQuota
			return nil
		}
		if usage.Status != "reserved" || !usage.PendingFinal || usage.AttemptedAt == 0 ||
			usage.PendingFinalAmount <= usage.Reserved || event.Ready || event.DeliveredAt != 0 ||
			(event.ResponseState != "" && event.ResponseState != "withheld") ||
			event.FinalQuota != usage.PendingFinalAmount || event.TeamId != usage.TeamId ||
			event.UserId != usage.MemberUserId {
			return errors.New("team request is not eligible for undelivered charge compensation")
		}
		if period.AmountUsed < usage.Reserved || period.AmountUsed > period.AmountTotal {
			return errors.New("invalid team period balance")
		}
		if period.AmountTotal-period.AmountUsed >= usage.PendingFinalAmount-usage.Reserved {
			return errors.New("original period can settle this charge")
		}
		details := map[string]any{}
		if event.BillingDetails != "" {
			if err := common.UnmarshalJsonStr(event.BillingDetails, &details); err != nil {
				return err
			}
		}
		details["platform_compensation"] = "provider_billed_result_not_delivered"
		details["compensation_info"] = map[string]any{"evidence_ref": evidenceRef, "reviewer_id": reviewerId}
		encoded, err := common.Marshal(details)
		if err != nil || len(encoded) > 16384 {
			return errors.New("invalid compensated team billing details")
		}
		if err := tx.Model(&period).Update("amount_used", gorm.Expr("amount_used - ?", usage.Reserved)).Error; err != nil {
			return err
		}
		if err := tx.Model(&usage).Updates(map[string]any{
			"status": "refunded", "pending_final": false,
			"resolved_by": reviewerId, "resolved_at": resolvedAt, "evidence_ref": evidenceRef,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&event).Updates(map[string]any{
			"response_state": "compensated_no_delivery", "billing_details": string(encoded),
		}).Error; err != nil {
			return err
		}
		unbilledQuota = event.FinalQuota
		applied = true
		return nil
	})
	return applied, unbilledQuota, err
}

// CreditTeamDeliveryDispute grants the original quota period a one-time
// operator-approved credit when a charged response was not received in full.
// Server write completion cannot prove client receipt, so evidence and a
// human decision are required. The provider usage, counters and consume log
// remain gross; this is an explicit credit, never an inferred billing refund.
// An expired quota period cannot be credited or carried into another period.
func CreditTeamDeliveryDispute(requestId string, reviewerId int, evidenceRef string) (bool, int64, error) {
	if requestId == "" || len(requestId) > 64 || reviewerId <= 0 || !validTeamEvidenceRef(evidenceRef) {
		return false, 0, errors.New("invalid team delivery dispute evidence")
	}
	applied := false
	var credited int64
	databaseNow := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&usage).Error; err != nil {
			return err
		}
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		var event TeamSyncBillingEvent
		if err := lockForUpdate(tx).Where("request_id = ?", requestId).First(&event).Error; err != nil {
			return err
		}
		// Do not query DB through the global handle while holding a transaction:
		// SQLite fixtures may have a single connection. Include time spent
		// waiting for locks, and reject conservatively if clocks disagree.
		now := max(databaseNow, common.GetTimestamp())
		if event.CreditedQuota > 0 {
			if event.CreditBy == reviewerId && event.CreditRef == evidenceRef &&
				event.CreditedQuota == usage.FinalAmount {
				credited = event.CreditedQuota
				return nil
			}
			return errors.New("team delivery dispute was already credited")
		}
		if now < period.StartTime || now >= period.EndTime {
			return errors.New("team quota period has expired or is not active; delivery dispute credit is unavailable")
		}
		if usage.Status != "settled" || usage.PendingFinal || usage.AttemptedAt == 0 ||
			usage.FinalAmount <= 0 || event.FinalQuota != usage.FinalAmount ||
			!event.Ready || event.TeamId != usage.TeamId || event.UserId != usage.MemberUserId ||
			period.AmountUsed < usage.FinalAmount || period.AmountUsed > period.AmountTotal ||
			event.ResponseState == "compensated_no_delivery" {
			return errors.New("team request is not eligible for delivery dispute credit")
		}
		if err := tx.Model(&period).Update("amount_used", gorm.Expr("amount_used - ?", usage.FinalAmount)).Error; err != nil {
			return err
		}
		if err := tx.Model(&event).Updates(map[string]any{
			"credited_quota": usage.FinalAmount, "credit_by": reviewerId,
			"credit_at": now, "credit_ref": evidenceRef,
		}).Error; err != nil {
			return err
		}
		credited = usage.FinalAmount
		applied = true
		return nil
	})
	return applied, credited, err
}

// FinalizeTeamTask commits the terminal task transition and its funding
// adjustment together. A failed adjustment leaves the task unfinished so the
// next poll can retry; a competing poller cannot settle the same task twice.
func FinalizeTeamTask(task *Task, from TaskStatus, finalAmount int64, reason string, clamp *common.QuotaClamp) (bool, error) {
	if task == nil || task.ID <= 0 || task.PrivateData.TeamId <= 0 || task.PrivateData.TeamRequestId == "" ||
		finalAmount < 0 || finalAmount > common.MaxQuota ||
		(task.Status != TaskStatusSuccess && task.Status != TaskStatusFailure) ||
		(task.Status == TaskStatusFailure && finalAmount != 0) {
		return false, errors.New("invalid team task finalization")
	}
	won := false
	eventTime := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var stored Task
		result := lockForUpdate(tx).Where("id = ? AND status = ?", task.ID, from).First(&stored)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		if result.Error != nil {
			return result.Error
		}
		var usage TeamUsage
		if err := lockForUpdate(tx).Where("request_id = ?", task.PrivateData.TeamRequestId).First(&usage).Error; err != nil {
			return err
		}
		if usage.Status != "reserved" || usage.TeamId != task.PrivateData.TeamId ||
			usage.MemberUserId != stored.UserId || stored.PrivateData.TeamRequestId != usage.RequestId ||
			stored.PrivateData.TeamId != usage.TeamId || int64(stored.Quota) != usage.Reserved {
			return errors.New("team task reservation does not match persisted task")
		}
		var period TeamQuotaPeriod
		if err := lockForUpdate(tx).First(&period, usage.PeriodId).Error; err != nil {
			return err
		}
		delta := finalAmount - usage.Reserved
		if delta > 0 && (delta > period.AmountTotal || period.AmountUsed > period.AmountTotal-delta) {
			return errors.New("team subscription quota insufficient")
		}
		if delta < 0 && period.AmountUsed < -delta {
			return errors.New("invalid team period balance")
		}
		if delta != 0 {
			updated := tx.Model(&TeamQuotaPeriod{}).Where("id = ? AND amount_used >= ? AND amount_used <= ?",
				period.Id, -min(delta, int64(0)), period.AmountTotal-max(delta, int64(0))).
				Update("amount_used", gorm.Expr("amount_used + ?", delta))
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return errors.New("team subscription quota insufficient")
			}
		}
		usage.Reserved = finalAmount
		if task.Status == TaskStatusSuccess {
			usage.Status = "settled"
			usage.FinalAmount = finalAmount
		} else {
			usage.Status = "refunded"
		}
		if err := tx.Save(&usage).Error; err != nil {
			return err
		}
		task.Quota = int(finalAmount)
		result = tx.Model(&Task{}).Where("id = ? AND status = ?", task.ID, from).Select("*").Updates(task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("team task terminal transition conflict")
		}
		// These denormalized counters live in the same database as the task.
		// Commit them with the quota and terminal state so a process crash
		// cannot lose the adjustment or replay it on a later poll.
		counterDelta := finalAmount - int64(stored.Quota)
		if counterDelta != 0 {
			if result := tx.Model(&User{}).Where("id = ?", stored.UserId).
				Update("used_quota", gorm.Expr("used_quota + ?", counterDelta)); result.Error != nil || result.RowsAffected != 1 {
				return errors.New("team task user counter adjustment failed")
			}
			if result := tx.Model(&Channel{}).Where("id = ?", stored.ChannelId).
				Update("used_quota", gorm.Expr("used_quota + ?", counterDelta)); result.Error != nil || result.RowsAffected != 1 {
				return errors.New("team task channel counter adjustment failed")
			}
			clampJSON := ""
			if clamp != nil {
				encoded, err := common.Marshal(clamp)
				if err != nil {
					return err
				}
				clampJSON = string(encoded)
			}
			modelName := stored.Properties.OriginModelName
			if stored.PrivateData.BillingContext != nil && stored.PrivateData.BillingContext.OriginModelName != "" {
				modelName = stored.PrivateData.BillingContext.OriginModelName
			}
			other := map[string]interface{}{"billing_source": "team", "team_id": usage.TeamId,
				"task_id": stored.TaskID, "pre_consumed_quota": stored.Quota, "actual_quota": finalAmount}
			if bc := stored.PrivateData.BillingContext; bc != nil {
				other["model_price"] = bc.ModelPrice
				if bc.ModelRatio > 0 {
					other["model_ratio"] = bc.ModelRatio
				}
				other["group_ratio"] = bc.GroupRatio
				for key, value := range bc.OtherRatios {
					other[key] = value
				}
			}
			if stored.Properties.UpstreamModelName != "" && stored.Properties.UpstreamModelName != stored.Properties.OriginModelName {
				other["is_model_mapped"] = true
				other["upstream_model_name"] = stored.Properties.UpstreamModelName
			}
			if err := tx.Create(&TeamTaskBillingEvent{TaskId: task.ID, PublicTaskId: stored.TaskID,
				TeamId: usage.TeamId, UserId: stored.UserId, ChannelId: stored.ChannelId,
				TokenId: stored.PrivateData.TokenId, Group: stored.Group, ModelName: modelName,
				PreviousQuota: int64(stored.Quota), FinalQuota: finalAmount,
				Reason: reason, ClampJSON: clampJSON, OtherJSON: common.MapToJsonStr(other), CreatedAt: eventTime}).Error; err != nil {
				return err
			}
		}
		won = true
		return nil
	})
	return won && err == nil, err
}

func TeamSubscriptionSummary(teamId int) (*TeamSubscription, *TeamQuotaPeriod, error) {
	now := GetDBTimestamp()
	var sub TeamSubscription
	var period *TeamQuotaPeriod
	err := DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", teamId, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		if err := tx.Where("team_id = ? AND status = ? AND start_time <= ? AND end_time > ?", teamId, TeamStatusActive, now, now).First(&sub).Error; err != nil {
			return err
		}
		var err error
		period, err = currentTeamPeriodTx(tx, &sub, now)
		return err
	})
	return &sub, period, err
}
