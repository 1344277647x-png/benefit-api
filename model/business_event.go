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
)

// BusinessEvent is a transactional outbox row. Domain transactions insert it
// in the primary database; delivery into the independently configured log
// database is idempotent through BusinessEventLogReceipt.
type BusinessEvent struct {
	Id          int64  `json:"id"`
	EventKey    string `json:"event_key" gorm:"type:varchar(160);uniqueIndex"`
	Category    string `json:"category" gorm:"type:varchar(24);index"`
	Action      string `json:"action" gorm:"type:varchar(64);index"`
	UserId      int    `json:"user_id" gorm:"index"`
	RequestId   string `json:"request_id" gorm:"type:varchar(64);index"`
	Content     string `json:"content" gorm:"type:varchar(255)"`
	DetailsJSON string `json:"-" gorm:"type:text"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;index"`
	DeliveredAt int64  `json:"delivered_at" gorm:"type:bigint;index"`
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
	err := DB.WithContext(ctx).Where("delivered_at = 0").Order("id asc").Limit(limit).Find(&events).Error
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

func MarkBusinessEventDelivered(ctx context.Context, eventKey string) error {
	return DB.WithContext(ctx).Model(&BusinessEvent{}).Where("event_key = ? AND delivered_at = 0", eventKey).
		Update("delivered_at", GetDBTimestamp()).Error
}
