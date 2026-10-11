package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Producers only persist immutable facts. Reporting workers never write wallets,
// tokens, subscriptions or payment orders. Receipts are retained for idempotency.
type OperationsEvent struct {
	Id            int64  `gorm:"primaryKey;index:idx_operations_pending,priority:3"`
	EventKey      string `gorm:"type:varchar(160);uniqueIndex"`
	Lane          string `gorm:"type:varchar(80);index:idx_operations_pending,priority:1"`
	RequestId     string `gorm:"type:varchar(64);index"`
	UserId        int
	Quota         int64  `gorm:"type:bigint"`
	Delta         int64  `gorm:"type:bigint"`
	Payload       string `gorm:"type:text"`
	CreatedAt     int64  `gorm:"type:bigint;index"`
	Done          bool   `gorm:"index:idx_operations_pending,priority:2;index:idx_operations_due,priority:1"`
	Attempts      int
	NextAttemptAt int64  `gorm:"type:bigint;index:idx_operations_due,priority:2"`
	LastError     string `gorm:"type:varchar(80)"`
}

type OperationsLane struct {
	Lane string `gorm:"primaryKey;type:varchar(80)"`
}

// No credentials, bodies or mutable model pointers belong in this snapshot.
type operationsFact struct {
	Action   string
	PoolKey  string
	Opening  int64
	Quota    int64
	Delta    int64
	UserId   int
	MemberId int
	Kind     string
	Money    float64
	Rate     float64
	Verified bool
	Gift     bool
	Balance  bool
	Charged  int64
	EndTime  int64
}

func enqueueOperationsTx(tx *gorm.DB, key, request string, f operationsFact) error {
	if !operationsAvailable(tx) {
		return nil
	}
	// Invalid optional reporting configuration means unknown income. It must
	// not cause JSON encoding to roll back a valid business transaction.
	if math.IsNaN(f.Rate) || math.IsInf(f.Rate, 0) || f.Rate <= 0 {
		f.Rate, f.Verified = 0, false
	}
	if math.IsNaN(f.Money) || math.IsInf(f.Money, 0) || f.Money < 0 {
		f.Money, f.Verified = 0, false
	}
	lane := OperationsLane{Lane: OperationsWalletKey(f.UserId)}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lane).Error; err != nil {
		return err
	}
	if err := lockForUpdate(tx).First(&lane, "lane = ?", lane.Lane).Error; err != nil {
		return err
	}
	data, err := common.Marshal(f)
	if err != nil {
		return err
	}
	e := OperationsEvent{EventKey: key, Lane: lane.Lane, RequestId: request, UserId: f.UserId, Quota: f.Quota, Delta: f.Delta, Payload: string(data), CreatedAt: common.GetTimestamp()}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&e)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var old OperationsEvent
		if err := tx.First(&old, "event_key = ?", key).Error; err != nil {
			return err
		}
		// Opening is deliberately first-write-wins, before any reservation.
		if f.Action != "open" && (old.Payload != e.Payload || old.RequestId != request) {
			return errors.New("accounting event idempotency conflict")
		}
	}
	return nil
}

func operationsSettlementExists(tx *gorm.DB, key string, user int, quota, delta int64, pool string) (bool, error) {
	var e OperationsEvent
	err := tx.First(&e, "event_key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var f operationsFact
	if common.UnmarshalJsonStr(e.Payload, &f) != nil || e.UserId != user || e.Quota != quota || e.Delta != delta || f.PoolKey != pool {
		return false, errors.New("settlement idempotency conflict")
	}
	return true, nil
}

func applyOperationsFactTx(tx *gorm.DB, e OperationsEvent) error {
	var f operationsFact
	if err := common.UnmarshalJsonStr(e.Payload, &f); err != nil {
		return err
	}
	if f.UserId <= 0 || f.Quota < 0 {
		return errors.New("invalid accounting fact")
	}
	open := func() error { return operationsOpenWalletTx(tx, f.UserId, f.Opening) }
	credit := func(key, kind, value, unknown string) error {
		return OperationsCreditTx(tx, f.PoolKey, key, kind, f.UserId, f.Quota, value, unknown, e.CreatedAt)
	}
	switch f.Action {
	case "open":
		return open()
	case "credit", "plan":
		value, unknown, kind := "0", strconv.FormatInt(f.Quota, 10), "unverified_credit"
		if f.Gift {
			unknown, kind = "0", "gift"
		}
		if f.Verified && f.Rate > 0 {
			value = decimal.NewFromFloat(f.Money).DivRound(decimal.NewFromFloat(f.Rate), 24).String()
			unknown, kind = "0", "receipt"
		}
		if f.Action == "credit" {
			if err := open(); err != nil {
				return err
			}
		}
		if f.Balance {
			if err := open(); err != nil {
				return err
			}
			entry, err := OperationsTakeTx(tx, OperationsWalletKey(f.UserId), "purchase:"+f.PoolKey, "", "transfer", f.UserId, f.Charged, e.CreatedAt)
			if err != nil {
				return err
			}
			value, kind, unknown = entry.ValueUSD, "transfer", "0"
			if f.Charged > 0 {
				u, err := decimal.NewFromString(entry.UnknownQuota)
				if err != nil {
					return err
				}
				unknown = u.Mul(decimal.NewFromInt(f.Quota)).DivRound(decimal.NewFromInt(f.Charged), 24).String()
			}
		}
		if err := credit(e.EventKey, kind, value, unknown); err != nil {
			return err
		}
		if f.Action == "plan" {
			return tx.Model(&OperationsPool{}).Where("pool_key = ?", f.PoolKey).Update("end_time", f.EndTime).Error
		}
		return nil
	case "usage", "adjustment":
		if f.PoolKey == OperationsWalletKey(f.UserId) {
			if err := open(); err != nil {
				return err
			}
			if err := operationsEnsureInventoryTx(tx, f.UserId, f.Quota, e.EventKey); err != nil {
				return err
			}
		} else {
			var count int64
			if err := tx.Model(&OperationsPool{}).Where("pool_key = ?", f.PoolKey).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return nil
			} // legacy/unlimited terms have no verified income
		}
		entry, err := OperationsTakeTx(tx, f.PoolKey, e.EventKey, e.RequestId, f.Kind, f.UserId, f.Quota, e.CreatedAt)
		if err != nil {
			return err
		}
		if f.MemberId > 0 {
			return tx.Model(&OperationsEntry{}).Where("event_key = ?", entry.EventKey).Update("request_user_id", f.MemberId).Error
		}
		return nil
	case "override":
		if err := open(); err != nil {
			return err
		}
		var pool OperationsPool
		if err := tx.First(&pool, "pool_key = ?", f.PoolKey).Error; err != nil {
			return err
		}
		if _, err := OperationsTakeTx(tx, pool.PoolKey, e.EventKey+":remove", "", "adjustment", f.UserId, pool.Quota, e.CreatedAt); err != nil {
			return err
		}
		return credit(e.EventKey+":fund", "unverified_credit", "0", strconv.FormatInt(f.Quota, 10))
	default:
		return errors.New("unknown accounting fact")
	}
}

// Only the first unfinished event in each funding lane is eligible. A failed
// lane is retained with backoff; other users continue, without reordering money.
func ProcessOperationsEvents(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var last error
	for remaining := limit; remaining > 0; {
		var events []OperationsEvent
		err := DB.WithContext(ctx).Where("done = ? AND next_attempt_at <= ?", false, common.GetTimestamp()).
			Where("NOT EXISTS (SELECT 1 FROM operations_events prior WHERE prior.lane = operations_events.lane AND prior.done = ? AND prior.id < operations_events.id)", false).
			Order("id ASC").Limit(remaining).Find(&events).Error
		if err != nil {
			return err
		}
		if len(events) == 0 {
			break
		}
		remaining -= len(events)
		for _, candidate := range events {
			err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var e OperationsEvent
				if err := lockForUpdate(tx).First(&e, candidate.Id).Error; err != nil {
					return err
				}
				if e.Done || e.NextAttemptAt > common.GetTimestamp() {
					return nil
				}
				// Savepoint recovers PostgreSQL's aborted transaction before marking
				// failure. Projection changes and the completion receipt are atomic.
				err := tx.Transaction(func(inner *gorm.DB) error { return applyOperationsFactTx(inner, e) })
				if err != nil {
					last = errors.New("accounting projection pending; inspect operations events")
					delay := int64(1 << min(e.Attempts+1, 12))
					common.SysError(fmt.Sprintf("accounting projection pending event=%d user=%d attempt=%d", e.Id, e.UserId, e.Attempts+1))
					alertErr := tx.Transaction(func(inner *gorm.DB) error {
						return enqueueBusinessEventTx(inner, &BusinessEvent{EventKey: fmt.Sprintf("operations-pending:%d", e.Id), Category: BusinessEventBilling, Action: "accounting_projection_failed", UserId: e.UserId, RequestId: e.RequestId, CreatedAt: common.GetTimestamp(), Content: "报表核算失败，业务扣费不受影响；待核算事件保留重试"}, map[string]any{"admin_info": map[string]any{"accounting_event_id": e.Id, "status": "pending_accounting", "quota": e.Quota, "wallet_delta": e.Delta, "billing_retry": false}})
					})
					if alertErr != nil {
						common.SysError("accounting alert delivery unavailable; durable pending event retained")
					}
					return tx.Model(&e).Updates(map[string]any{"attempts": e.Attempts + 1, "next_attempt_at": common.GetTimestamp() + delay, "last_error": "projection_failed"}).Error
				}
				return tx.Model(&e).Updates(map[string]any{"done": true, "last_error": "", "next_attempt_at": 0}).Error
			})
			if err != nil {
				last = err
			}
		}
	}
	return last
}
