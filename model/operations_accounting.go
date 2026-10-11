package model

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Monetary values are decimal strings, preserving precision across all supported
// databases. These records describe economic value, never spendable wallet quota.
type OperationsPool struct {
	PoolKey         string `gorm:"primaryKey;type:varchar(80)" json:"pool_key"`
	UserId          int    `gorm:"index" json:"user_id"`
	Quota           int64  `gorm:"type:bigint" json:"quota"`
	ValueUSD        string `gorm:"type:varchar(96)" json:"value_usd"`
	UnknownQuota    string `gorm:"type:varchar(96)" json:"unknown_quota"`
	EndTime         int64  `gorm:"type:bigint;index" json:"end_time"`
	Expired         bool   `json:"expired"`
	ExpiryCheckedAt int64  `gorm:"type:bigint;not null;default:0;index" json:"-"`
	CreatedAt       int64  `gorm:"type:bigint" json:"created_at"`
}

type OperationsEntry struct {
	RequestUserId int    `gorm:"index" json:"request_user_id"`
	EventKey      string `gorm:"primaryKey;type:varchar(160)" json:"event_key"`
	PoolKey       string `gorm:"type:varchar(80);index" json:"pool_key"`
	UserId        int    `gorm:"index" json:"user_id"`
	RequestId     string `gorm:"type:varchar(64);index" json:"request_id"`
	Kind          string `gorm:"type:varchar(24);index" json:"kind"`
	Quota         int64  `gorm:"type:bigint" json:"quota"`
	ValueUSD      string `gorm:"type:varchar(96)" json:"value_usd"`
	UnknownQuota  string `gorm:"type:varchar(96)" json:"unknown_quota"`
	CreatedAt     int64  `gorm:"type:bigint;index" json:"created_at"`
}

type ChannelCostVersion struct {
	Id          int64  `json:"id"`
	ChannelId   int    `gorm:"uniqueIndex:idx_cost_effective,priority:1" json:"channel_id"`
	Multiplier  string `gorm:"type:varchar(48)" json:"multiplier"`
	EffectiveAt int64  `gorm:"type:bigint;uniqueIndex:idx_cost_effective,priority:2" json:"effective_at"`
	CreatedAt   int64  `gorm:"type:bigint" json:"created_at"`
	OperatorId  int    `json:"operator_id"`
}

func SaveChannelCostVersion(channelId, operatorId int, multiplier string, effectiveAt int64) (*ChannelCostVersion, error) {
	if len(multiplier) == 0 || len(multiplier) > 32 || strings.ContainsAny(multiplier, "eE") {
		return nil, errors.New("invalid cost multiplier")
	}
	ratio, err := decimal.NewFromString(multiplier)
	now := common.GetTimestamp()
	if err != nil || len(multiplier) > 32 || ratio.IsNegative() || ratio.GreaterThan(decimal.NewFromInt(1000)) || ratio.Exponent() < -8 {
		return nil, errors.New("cost multiplier must be between 0 and 1000 with at most 8 decimals")
	}
	if channelId <= 0 || operatorId <= 0 || effectiveAt < now || effectiveAt > now+366*86400 {
		return nil, errors.New("invalid cost effective time; historical versions cannot be overwritten")
	}
	v := &ChannelCostVersion{ChannelId: channelId, OperatorId: operatorId, Multiplier: ratio.String(), EffectiveAt: effectiveAt, CreatedAt: now}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id").First(&channel, channelId).Error; err != nil {
			return err
		}
		if err := tx.Create(v).Error; err != nil {
			return err
		}
		return enqueueBusinessEventTx(tx, &BusinessEvent{EventKey: fmt.Sprintf("operations:cost:%d", v.Id), Category: BusinessEventOperations, Action: "operations.cost_version", UserId: operatorId, Content: "Channel cost multiplier version created", CreatedAt: now}, map[string]any{"channel_id": channelId, "multiplier": v.Multiplier, "effective_at": effectiveAt, "version_id": v.Id})
	})
	return v, err
}

func operationsAmounts(pool OperationsPool) (decimal.Decimal, decimal.Decimal, error) {
	value, err := decimal.NewFromString(pool.ValueUSD)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	unknown, err := decimal.NewFromString(pool.UnknownQuota)
	// Older proportional withdrawals divided before multiplying and could leave
	// a sub-1e-18 excess in an entirely unknown pool. Repair only that rounding
	// residue; materially invalid inventory must still fail closed.
	limit := decimal.NewFromInt(pool.Quota)
	if err == nil && unknown.GreaterThan(limit) && unknown.Sub(limit).LessThanOrEqual(decimal.New(1, -18)) {
		unknown = limit
	}
	if err != nil || pool.Quota < 0 || value.IsNegative() || unknown.IsNegative() || unknown.GreaterThan(decimal.NewFromInt(pool.Quota)) {
		return decimal.Zero, decimal.Zero, errors.New("invalid accounting pool")
	}
	return value, unknown, nil
}

// OperationsTakeTx proportionally removes known economic value and unknown
// historical quota. Remainders stay in the pool; its final withdrawal is exact.
// The caller holds the user's row lock before calling, including on first use.
func OperationsTakeTx(tx *gorm.DB, poolKey, eventKey, requestId, kind string, userId int, quota int64, at int64) (*OperationsEntry, error) {
	// A savepoint also protects callers already inside a business transaction.
	var entry *OperationsEntry
	err := tx.Transaction(func(inner *gorm.DB) error {
		var err error
		entry, err = operationsTakeLocked(inner, poolKey, eventKey, requestId, kind, userId, quota, at)
		return err
	})
	return entry, err
}

func operationsTakeLocked(tx *gorm.DB, poolKey, eventKey, requestId, kind string, userId int, quota int64, at int64) (*OperationsEntry, error) {
	if quota < 0 || eventKey == "" || userId <= 0 {
		return nil, errors.New("invalid accounting withdrawal")
	}
	var existing OperationsEntry
	if err := tx.First(&existing, "event_key = ?", eventKey).Error; err == nil {
		if existing.PoolKey != poolKey || existing.UserId != userId || existing.Quota != quota || existing.Kind != kind || existing.RequestId != requestId {
			return nil, errors.New("accounting idempotency conflict")
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var pool OperationsPool
	if err := lockForUpdate(tx).First(&pool, "pool_key = ?", poolKey).Error; err != nil {
		return nil, err
	}
	if pool.UserId != userId || pool.Expired || quota > pool.Quota {
		return nil, errors.New("accounting inventory unavailable")
	}
	value, unknown, err := operationsAmounts(pool)
	if err != nil {
		return nil, err
	}
	usedValue, usedUnknown := decimal.Zero, decimal.Zero
	if quota == pool.Quota {
		usedValue, usedUnknown = value, unknown
	} else if quota > 0 {
		amount, total := decimal.NewFromInt(quota), decimal.NewFromInt(pool.Quota)
		usedValue = value.Mul(amount).DivRound(total, 24)
		usedUnknown = unknown.Mul(amount).DivRound(total, 24)
		// Keep both the withdrawal and remaining unknown inventory within their
		// quota bounds even for mixed pools at the rounding boundary.
		usedUnknown = decimal.Min(unknown, decimal.Min(amount, usedUnknown))
		usedUnknown = decimal.Max(usedUnknown, unknown.Sub(decimal.NewFromInt(pool.Quota-quota)))
	}
	entry := &OperationsEntry{EventKey: eventKey, PoolKey: poolKey, UserId: userId, RequestId: requestId, Kind: kind, Quota: quota, ValueUSD: usedValue.String(), UnknownQuota: usedUnknown.String(), CreatedAt: at}
	pool.Quota -= quota
	pool.ValueUSD = value.Sub(usedValue).String()
	pool.UnknownQuota = unknown.Sub(usedUnknown).String()
	if err := tx.Save(&pool).Error; err != nil {
		return nil, err
	}
	return entry, tx.Create(entry).Error
}

func OperationsCreditTx(tx *gorm.DB, poolKey, eventKey, kind string, userId int, quota int64, valueUSD, unknownQuota string, at int64) error {
	return tx.Transaction(func(inner *gorm.DB) error {
		return operationsCreditLocked(inner, poolKey, eventKey, kind, userId, quota, valueUSD, unknownQuota, at)
	})
}

func operationsCreditLocked(tx *gorm.DB, poolKey, eventKey, kind string, userId int, quota int64, valueUSD, unknownQuota string, at int64) error {
	if len(valueUSD) == 0 || len(valueUSD) > 80 || len(unknownQuota) == 0 || len(unknownQuota) > 80 || strings.ContainsAny(valueUSD+unknownQuota, "eE") {
		return errors.New("invalid accounting amount")
	}
	value, err := decimal.NewFromString(valueUSD)
	if err != nil {
		return err
	}
	unknown, err := decimal.NewFromString(unknownQuota)
	if err != nil || value.IsNegative() || unknown.IsNegative() || unknown.GreaterThan(decimal.NewFromInt(quota)) || quota < 0 || userId <= 0 || eventKey == "" {
		return errors.New("invalid accounting credit")
	}
	var existing OperationsEntry
	if err := tx.First(&existing, "event_key = ?", eventKey).Error; err == nil {
		if existing.PoolKey != poolKey || existing.UserId != userId || existing.Quota != quota || existing.ValueUSD != value.String() || existing.UnknownQuota != unknown.String() || existing.Kind != kind {
			return errors.New("accounting idempotency conflict")
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&OperationsPool{PoolKey: poolKey, UserId: userId, ValueUSD: "0", UnknownQuota: "0", CreatedAt: at}).Error; err != nil {
		return err
	}
	var pool OperationsPool
	if err := lockForUpdate(tx).First(&pool, "pool_key = ?", poolKey).Error; err != nil {
		return err
	}
	oldValue, oldUnknown, err := operationsAmounts(pool)
	if err != nil {
		return err
	}
	if pool.UserId != userId || pool.Expired || quota > int64(common.MaxWalletQuota)-pool.Quota {
		return errors.New("invalid accounting pool credit")
	}
	pool.Quota += quota
	pool.ValueUSD = oldValue.Add(value).String()
	pool.UnknownQuota = oldUnknown.Add(unknown).String()
	if err := tx.Save(&pool).Error; err != nil {
		return err
	}
	return tx.Create(&OperationsEntry{EventKey: eventKey, PoolKey: poolKey, UserId: userId, Kind: kind, Quota: quota, ValueUSD: value.String(), UnknownQuota: unknown.String(), CreatedAt: at}).Error
}

// Base cost is reconstructed only when the saved billing information is enough
// to establish identical pricing. Complex expressions/tools/media stay unknown.
func OperationsEstimateCost(quota int, other map[string]any, multiplier string) (float64, bool) {
	if quota < 0 || common.QuotaPerUnit <= 0 {
		return 0, false
	}
	for _, key := range []string{"billing_mode", "tool_surcharges", "web_search", "file_search", "image_generation_call", "quota_saturation"} {
		if v, ok := other[key]; ok && v != nil && v != false && v != "" {
			return 0, false
		}
	}
	if admin, ok := other["admin_info"].(map[string]interface{}); ok && admin["quota_saturation"] != nil {
		return 0, false
	}
	if admin, ok := other["admin_info"].(map[string]interface{}); ok {
		if attempts, ok := admin["use_channel"].([]interface{}); ok && len(attempts) > 1 {
			return 0, false // Earlier attempts may have incurred additional costs.
		}
	}
	ratioValue, exists := other["group_ratio"]
	if special, ok := other["user_group_ratio"]; ok {
		// -1 is the existing log format's sentinel for no special price.
		parsed, err := decimal.NewFromString(fmt.Sprint(special))
		if err != nil {
			return 0, false
		}
		if !parsed.Equal(decimal.NewFromInt(-1)) {
			ratioValue, exists = special, true
		}
	}
	if !exists {
		return 0, false
	}
	group, err := decimal.NewFromString(fmt.Sprint(ratioValue))
	if err != nil || !group.IsPositive() {
		return 0, false
	}
	mult, err := decimal.NewFromString(multiplier)
	if err != nil || mult.IsNegative() {
		return 0, false
	}
	cost, _ := decimal.NewFromInt(int64(quota)).Div(decimal.NewFromFloat(common.QuotaPerUnit)).Div(group).Mul(mult).Float64()
	return cost, !math.IsNaN(cost) && !math.IsInf(cost, 0) && cost >= 0
}

func OperationsWalletKey(userId int) string { return "wallet:" + strconv.Itoa(userId) }

// An expiry event owns the remaining value, never an arbitrary channel.
func ExpireOperationsPoolTx(tx *gorm.DB, key string, now int64) error {
	var pool OperationsPool
	if err := lockForUpdate(tx).First(&pool, "pool_key = ?", key).Error; err != nil {
		return err
	}
	if pool.Expired {
		return nil
	}
	if pool.EndTime <= 0 || now < pool.EndTime {
		return errors.New("accounting term has not expired")
	}
	_, err := OperationsTakeTx(tx, key, "expiry:"+key, "", "expiry", pool.UserId, pool.Quota, pool.EndTime)
	if err != nil {
		return err
	}
	return tx.Model(&OperationsPool{}).Where("pool_key = ?", key).Update("expired", true).Error
}

func OperationsBusinessDay(timestamp int64) string {
	return time.Unix(timestamp, 0).In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}
