/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

const unsettledPreConsumeGraceSeconds int64 = 300

var billingReconciliationEventActions = []string{
	"settlement_failed",
	"partial_settlement_failed",
	"accounting_projection_failed",
}

// OpsQueueSummary reports read-only counts for background outbox queues and
// unresolved billing anomalies shown on the administrator overview dashboard.
type OpsQueueSummary struct {
	OperationsPending         int64 `json:"operations_pending"`
	OperationsRetrying        int64 `json:"operations_retrying"`
	OperationsOldestPendingAt int64 `json:"operations_oldest_pending_at"`
	BusinessPending           int64 `json:"business_pending"`
	BusinessRetrying          int64 `json:"business_retrying"`
	BusinessDeadLetter        int64 `json:"business_dead_letter"`
	BillingExceptionsCount    int64 `json:"billing_exceptions_count"`
	UnsettledPreConsumeCount  int64 `json:"unsettled_pre_consume_count"`
}

func GetOpsQueueSummary(ctx context.Context) (OpsQueueSummary, error) {
	var summary OpsQueueSummary
	if DB == nil {
		return summary, nil
	}
	db := DB.WithContext(ctx)

	if db.Migrator().HasTable(&OperationsEvent{}) {
		if err := db.Model(&OperationsEvent{}).Where("done = ?", false).Count(&summary.OperationsPending).Error; err != nil {
			return summary, err
		}
		if err := db.Model(&OperationsEvent{}).Where("done = ? AND attempts > 0", false).Count(&summary.OperationsRetrying).Error; err != nil {
			return summary, err
		}
		if summary.OperationsPending > 0 {
			var oldest int64
			if err := db.Model(&OperationsEvent{}).
				Select("COALESCE(MIN(created_at), 0)").
				Where("done = ?", false).
				Scan(&oldest).Error; err != nil {
				return summary, err
			}
			summary.OperationsOldestPendingAt = oldest
		}
	}

	if db.Migrator().HasTable(&BusinessEvent{}) {
		if err := db.Model(&BusinessEvent{}).
			Where("delivered_at = 0 AND dead_letter_at = 0").
			Count(&summary.BusinessPending).Error; err != nil {
			return summary, err
		}
		if err := db.Model(&BusinessEvent{}).
			Where("delivered_at = 0 AND dead_letter_at = 0 AND attempts > 0").
			Count(&summary.BusinessRetrying).Error; err != nil {
			return summary, err
		}
		if err := db.Model(&BusinessEvent{}).
			Where("dead_letter_at > 0").
			Count(&summary.BusinessDeadLetter).Error; err != nil {
			return summary, err
		}
		if err := db.Model(&BusinessEvent{}).
			Where("category = ? AND action IN ?", BusinessEventBilling, []string{ReconciliationKindSettlementFailed, ReconciliationKindPartialSettlementFailed}).
			Count(&summary.BillingExceptionsCount).Error; err != nil {
			return summary, err
		}
	}

	summary.BillingExceptionsCount += summary.OperationsRetrying // Only projections still awaiting a successful retry.
	if db.Migrator().HasTable(&SubscriptionPreConsumeRecord{}) {
		cutoff := common.GetTimestamp() - unsettledPreConsumeGraceSeconds
		if err := db.Model(&SubscriptionPreConsumeRecord{}).
			Where("status = ? AND created_at <= ?", "consumed", cutoff).
			Count(&summary.UnsettledPreConsumeCount).Error; err != nil {
			return summary, err
		}
	}

	return summary, nil
}
