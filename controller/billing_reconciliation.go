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
package controller

import (
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func parseBillingReconciliationParams(c *gin.Context) (model.BillingReconciliationParams, error) {
	now := time.Now().Unix()
	start := now - 30*24*60*60
	end := now
	var err error
	if value := strings.TrimSpace(c.Query("start_timestamp")); value != "" {
		start, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return model.BillingReconciliationParams{}, errors.New("invalid start_timestamp")
		}
	}
	if value := strings.TrimSpace(c.Query("end_timestamp")); value != "" {
		end, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return model.BillingReconciliationParams{}, errors.New("invalid end_timestamp")
		}
	}
	if start <= 0 || end <= 0 || end < start {
		return model.BillingReconciliationParams{}, errors.New("invalid reconciliation time range")
	}
	if end-start > 366*24*60*60 {
		return model.BillingReconciliationParams{}, errors.New("reconciliation time range cannot exceed 366 days")
	}
	userID := 0
	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		userID, err = strconv.Atoi(value)
		if err != nil || userID <= 0 {
			return model.BillingReconciliationParams{}, errors.New("invalid user_id")
		}
	}
	limit := 0
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 {
			return model.BillingReconciliationParams{}, errors.New("invalid limit")
		}
	}
	kind, err := model.NormalizeBillingReconciliationKind(c.Query("kind"))
	if err != nil {
		return model.BillingReconciliationParams{}, err
	}
	return model.BillingReconciliationParams{
		StartTimestamp: start,
		EndTimestamp:   end,
		UserID:         userID,
		RequestID:      strings.TrimSpace(c.Query("request_id")),
		Kind:           kind,
		Limit:          limit,
	}, nil
}

func GetBillingReconciliation(c *gin.Context) {
	params, err := parseBillingReconciliationParams(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	report, err := model.GetBillingReconciliation(c.Request.Context(), params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.EqualFold(c.Query("format"), "csv") {
		writeBillingReconciliationCSV(c, report)
		return
	}
	if strings.EqualFold(c.Query("download"), "true") {
		encoded, err := common.Marshal(report)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename=benefit-api-billing-reconciliation.json")
		c.Data(http.StatusOK, "application/json; charset=utf-8", encoded)
		return
	}
	common.ApiSuccess(c, report)
}

func writeBillingReconciliationCSV(c *gin.Context, report *model.BillingReconciliationReport) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=benefit-api-billing-reconciliation.csv")
	writer := csv.NewWriter(c.Writer)
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = writer.Write([]string{
		"source", "kind", "request_id", "event_key", "user_id", "username",
		"subscription_id", "funding_source", "model_name", "failure_stage", "status",
		"pre_consumed_quota", "actual_quota", "funding_diff_quota", "projected_quota",
		"pre_consumed_usd", "actual_usd", "funding_diff_usd",
		"has_consume_log", "delivered_at", "dead_letter_at", "created_at", "summary", "token_diff_quota",
	})
	for _, item := range report.Items {
		actualQuota, diffQuota, projectedQuota, tokenDiffQuota := "", "", "", ""
		actualUSD, diffUSD := "", ""
		if item.ActualQuota != nil {
			actualQuota = strconv.FormatInt(*item.ActualQuota, 10)
		}
		if item.DiffQuota != nil {
			diffQuota = strconv.FormatInt(*item.DiffQuota, 10)
		}
		if item.ProjectedQuota != nil {
			projectedQuota = strconv.FormatInt(*item.ProjectedQuota, 10)
		}
		if item.TokenDiffQuota != nil {
			tokenDiffQuota = strconv.FormatInt(*item.TokenDiffQuota, 10)
		}
		if item.ActualUSD != nil {
			actualUSD = strconv.FormatFloat(*item.ActualUSD, 'f', 6, 64)
		}
		if item.DiffUSD != nil {
			diffUSD = strconv.FormatFloat(*item.DiffUSD, 'f', 6, 64)
		}
		_ = writer.Write([]string{
			operationsCSVText(item.Source),
			operationsCSVText(item.Kind),
			operationsCSVText(item.RequestId),
			operationsCSVText(item.EventKey),
			strconv.Itoa(item.UserId),
			operationsCSVText(item.Username),
			strconv.Itoa(item.SubscriptionId),
			operationsCSVText(item.FundingSource),
			operationsCSVText(item.ModelName),
			operationsCSVText(item.FailureStage),
			operationsCSVText(item.Status),
			strconv.FormatInt(item.PreConsumedQuota, 10),
			actualQuota,
			diffQuota,
			projectedQuota,
			strconv.FormatFloat(item.PreConsumedUSD, 'f', 6, 64),
			actualUSD,
			diffUSD,
			strconv.FormatBool(item.HasConsumeLog),
			strconv.FormatInt(item.DeliveredAt, 10),
			strconv.FormatInt(item.DeadLetterAt, 10),
			strconv.FormatInt(item.CreatedAt, 10),
			operationsCSVText(item.Summary),
			tokenDiffQuota,
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		common.SysLog("failed to write billing reconciliation csv: " + err.Error())
	}
}
