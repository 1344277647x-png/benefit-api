package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BusinessEventReferral = "referral"
	BusinessEventLottery  = "lottery"
	BusinessEventTeam     = "team"
	BusinessEventBilling  = "billing"
)

// BusinessEvent is a transactional outbox row. Domain transactions insert it
// in the primary database; delivery into the independently configured log
// database is idempotent through BusinessEventLogReceipt.
type BusinessEvent struct {
	Id            int64  `json:"id"`
	EventKey      string `json:"event_key" gorm:"type:varchar(160);uniqueIndex"`
	Category      string `json:"category" gorm:"type:varchar(24);index"`
	Action        string `json:"action" gorm:"type:varchar(64);index"`
	UserId        int    `json:"user_id" gorm:"index"`
	RequestId     string `json:"request_id" gorm:"type:varchar(64);index"`
	Content       string `json:"content" gorm:"type:varchar(255)"`
	DetailsJSON   string `json:"-" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;index"`
	DeliveredAt   int64  `json:"delivered_at" gorm:"type:bigint;index"`
	Attempts      int    `json:"attempts" gorm:"not null;default:0"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"type:bigint;not null;default:0;index"`
	DeadLetterAt  int64  `json:"dead_letter_at" gorm:"type:bigint;not null;default:0;index"`
}

type BusinessEventLogReceipt struct {
	EventKey  string `gorm:"primaryKey;type:varchar(160)"`
	CreatedAt int64  `gorm:"type:bigint"`
}

func enqueueBusinessEventTx(tx *gorm.DB, event *BusinessEvent, details map[string]any) error {
	if tx == nil || event == nil || event.EventKey == "" || event.UserId <= 0 {
		return errors.New("invalid business event")
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = GetDBTimestamp()
	}
	event.DetailsJSON = common.MapToJsonStr(details)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error
}

func PendingBusinessEvents(ctx context.Context, limit int) ([]BusinessEvent, error) {
	if limit <= 0 || limit > 200 {
		return nil, errors.New("invalid business event batch size")
	}
	var events []BusinessEvent
	err := DB.WithContext(ctx).Where("delivered_at = 0 AND dead_letter_at = 0 AND next_attempt_at <= ?", GetDBTimestamp()).Order("id asc").Limit(limit).Find(&events).Error
	return events, err
}

func DeliverBusinessEvent(ctx context.Context, event BusinessEvent) error {
	logType := LogTypeUnknown
	switch event.Category {
	case BusinessEventReferral:
		logType = LogTypeReferral
	case BusinessEventLottery:
		logType = LogTypeLottery
	case BusinessEventTeam:
		logType = LogTypeTeam
	case BusinessEventBilling:
		logType = LogTypeError
	default:
		return errors.New("unsupported business event category")
	}
	username, _ := GetUsernameById(event.UserId, false)
	other := map[string]any{}
	if event.DetailsJSON != "" {
		if err := common.UnmarshalJsonStr(event.DetailsJSON, &other); err != nil {
			return err
		}
	}
	if other == nil {
		return errors.New("business event details must be an object")
	}
	other["business_event"] = map[string]any{"category": event.Category, "action": event.Action, "event_key": event.EventKey}
	return LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&BusinessEventLogReceipt{EventKey: event.EventKey, CreatedAt: common.GetTimestamp()})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return tx.Create(&Log{UserId: event.UserId, Username: username, CreatedAt: event.CreatedAt,
			Type: logType, Content: event.Content, RequestId: event.RequestId, Other: common.MapToJsonStr(other)}).Error
	})
}

// Accounting exceptions use the existing durable outbox even when error-log
// recording is disabled. Delivery to the separate log database can be retried.
func EnqueueBillingException(event *BusinessEvent, details map[string]any) error {
	if event == nil || event.Category != BusinessEventBilling {
		return errors.New("invalid billing exception")
	}
	// Avoid querying the global DB inside a transaction on single-connection
	// SQLite. This timestamp is metadata, not a quota-period boundary.
	if event.CreatedAt == 0 {
		event.CreatedAt = common.GetTimestamp()
	}
	return DB.Transaction(func(tx *gorm.DB) error { return enqueueBusinessEventTx(tx, event, details) })
}

// Keep failures recoverable without copying potentially sensitive payloads into
// diagnostic columns. A failed event must not hold up unrelated business logs.
func MarkBusinessEventFailed(ctx context.Context, event BusinessEvent) error {
	attempts := event.Attempts + 1
	now := GetDBTimestamp()
	updates := map[string]any{"attempts": attempts, "next_attempt_at": now + int64(min(attempts, 10))*60}
	if attempts >= 10 {
		updates["dead_letter_at"] = now
	}
	return DB.WithContext(ctx).Model(&BusinessEvent{}).
		Where("id = ? AND delivered_at = 0 AND attempts = ?", event.Id, event.Attempts).Updates(updates).Error
}

func MarkBusinessEventDelivered(ctx context.Context, eventKey string) error {
	return DB.WithContext(ctx).Model(&BusinessEvent{}).Where("event_key = ? AND delivered_at = 0", eventKey).
		Update("delivered_at", GetDBTimestamp()).Error
}
