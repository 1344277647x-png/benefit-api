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
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func parseOperationsReportParams(c *gin.Context) (model.OperationsReportParams, error) {
	now := time.Now().Unix()
	start := now - 30*24*60*60
	end := now
	var err error
	if value := strings.TrimSpace(c.Query("start_timestamp")); value != "" {
		start, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return model.OperationsReportParams{}, err
		}
	}
	if value := strings.TrimSpace(c.Query("end_timestamp")); value != "" {
		end, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return model.OperationsReportParams{}, err
		}
	}
	if start <= 0 || end <= 0 || end < start {
		return model.OperationsReportParams{}, &operationsReportInputError{message: "invalid report time range"}
	}
	if end-start > 366*24*60*60 {
		return model.OperationsReportParams{}, &operationsReportInputError{message: "report time range cannot exceed 366 days"}
	}
	channelID := 0
	if value := strings.TrimSpace(c.Query("channel_id")); value != "" {
		channelID, err = strconv.Atoi(value)
		if err != nil || channelID < 0 {
			return model.OperationsReportParams{}, &operationsReportInputError{message: "invalid channel_id"}
		}
	}
	return model.OperationsReportParams{
		StartTimestamp: start,
		EndTimestamp:   end,
		ModelName:      strings.TrimSpace(c.Query("model_name")),
		Username:       strings.TrimSpace(c.Query("username")),
		Group:          strings.TrimSpace(c.Query("group")),
		ChannelID:      channelID,
		GroupBy:        c.DefaultQuery("group_by", "day"),
	}, nil
}

type operationsReportInputError struct {
	message string
}

func (e *operationsReportInputError) Error() string { return e.message }

func enrichOperationsReport(report *model.OperationsReport) {
	if report == nil || len(report.ChannelIDs) == 0 {
		return
	}
	channels, err := model.GetChannelsByIds(report.ChannelIDs)
	if err != nil {
		report.Warnings = append(report.Warnings, "channel balance data is unavailable")
	} else {
		report.Channels = make([]model.OperationsReportChannel, 0, len(channels))
		channelNames := make(map[int]string, len(channels))
		for _, channel := range channels {
			if channel == nil {
				continue
			}
			report.Channels = append(report.Channels, model.OperationsReportChannel{
				ChannelID:    channel.Id,
				Name:         channel.Name,
				Balance:      channel.Balance,
				UsedQuota:    channel.UsedQuota,
				Status:       channel.Status,
				ResponseTime: channel.ResponseTime,
			})
			channelNames[channel.Id] = channel.Name
		}
		for index := range report.Rows {
			if name := channelNames[report.Rows[index].ChannelID]; name != "" {
				report.Rows[index].ChannelName = name
			}
		}
	}

	healthViews, healthErr := service.GetChannelHealthViews(time.Now())
	if healthErr != nil {
		report.Warnings = append(report.Warnings, "channel health data is unavailable")
		return
	}
	idSet := make(map[int]struct{}, len(report.ChannelIDs))
	for _, id := range report.ChannelIDs {
		idSet[id] = struct{}{}
	}
	for _, view := range healthViews {
		if _, ok := idSet[view.ChannelID]; !ok {
			continue
		}
		report.Health = append(report.Health, model.OperationsReportHealth{
			ChannelID:        view.ChannelID,
			ModelName:        view.ModelName,
			Status:           view.Status,
			RequestCount:     view.RequestCount,
			SuccessRate:      view.SuccessRate,
			AverageLatencyMs: view.AverageLatencyMs,
			AverageTTFTMs:    view.AverageTTFTMs,
		})
	}
}

func GetOperationsReport(c *gin.Context) {
	params, err := parseOperationsReportParams(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	report, err := model.GetOperationsReport(params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	enrichOperationsReport(report)
	if strings.EqualFold(c.Query("format"), "csv") {
		writeOperationsReportCSV(c, report)
		return
	}
	common.ApiSuccess(c, report)
}

func writeOperationsReportCSV(c *gin.Context, report *model.OperationsReport) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=benefit-api-operations-report.csv")
	writer := csv.NewWriter(c.Writer)
	// UTF-8 BOM keeps Chinese labels readable in spreadsheet applications.
	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	_ = writer.Write([]string{
		"key", "label", "model", "channel_id", "channel", "user_id", "username", "group",
		"requests", "successes", "errors", "tokens", "revenue_quota", "refund_quota",
		"revenue_usd", "refund_usd", "cost_usd", "cost_status", "unknown_cost_requests", "profit_usd", "error_rate",
	})
	for _, row := range report.Rows {
		profit := ""
		if row.ProfitUSD != nil {
			profit = strconv.FormatFloat(*row.ProfitUSD, 'f', 6, 64)
		}
		_ = writer.Write([]string{
			row.Key,
			row.Label,
			row.ModelName,
			strconv.Itoa(row.ChannelID),
			row.ChannelName,
			strconv.Itoa(row.UserID),
			row.Username,
			row.Group,
			strconv.FormatInt(row.RequestCount, 10),
			strconv.FormatInt(row.SuccessCount, 10),
			strconv.FormatInt(row.ErrorCount, 10),
			strconv.FormatInt(row.TokenCount, 10),
			strconv.FormatInt(row.RevenueQuota, 10),
			strconv.FormatInt(row.RefundQuota, 10),
			strconv.FormatFloat(row.RevenueUSD, 'f', 6, 64),
			strconv.FormatFloat(row.RefundUSD, 'f', 6, 64),
			strconv.FormatFloat(row.CostUSD, 'f', 6, 64),
			row.CostStatus,
			strconv.FormatInt(row.UnknownCostRequests, 10),
			profit,
			strconv.FormatFloat(row.ErrorRate, 'f', 4, 64),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		// Headers may already be committed; log the failure without replacing a
		// partially streamed CSV with a second response body.
		common.SysLog("failed to write operations report csv: " + err.Error())
	}
}
