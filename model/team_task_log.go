package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TeamTaskLogDeliverySupported() bool {
	return DB != nil && LOG_DB != nil && common.LogConsumeEnabled &&
		!common.UsingLogDatabase(common.DatabaseTypeClickHouse)
}

func PendingTeamTaskBillingEvents(ctx context.Context, limit int) ([]TeamTaskBillingEvent, error) {
	if limit <= 0 || limit > 100 {
		return nil, errors.New("invalid team billing event batch size")
	}
	var events []TeamTaskBillingEvent
	err := DB.WithContext(ctx).Where("delivered_at = 0").Order("task_id asc").Limit(limit).Find(&events).Error
	return events, err
}

func GetTeamTaskBillingEvent(ctx context.Context, taskId int64) (*TeamTaskBillingEvent, error) {
	var event TeamTaskBillingEvent
	if err := DB.WithContext(ctx).First(&event, "task_id = ?", taskId).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

// A receipt and its log are inserted on the same transactional log database.
// If the process dies before MarkTeamTaskBillingEventDelivered, retry observes
// the receipt and cannot insert a second log. ClickHouse is not supported.
func DeliverTeamTaskBillingLog(ctx context.Context, event TeamTaskBillingEvent, log *Log) error {
	if !TeamTaskLogDeliverySupported() || event.TaskId <= 0 {
		return errors.New("transactional team task log delivery unavailable")
	}
	receiptTime := common.GetTimestamp()
	return LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		receipt := TeamTaskLogReceipt{TaskId: event.TaskId, CreatedAt: receiptTime}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if log != nil {
			log.RequestId = fmt.Sprintf("team-task-%d", event.TaskId)
			return tx.Create(log).Error
		}
		return nil
	})
}

func MarkTeamTaskBillingEventDelivered(ctx context.Context, taskId int64) error {
	return DB.WithContext(ctx).Model(&TeamTaskBillingEvent{}).
		Where("task_id = ? AND delivered_at = 0", taskId).
		Update("delivered_at", GetDBTimestamp()).Error
}
