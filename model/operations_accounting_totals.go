package model

import "github.com/shopspring/decimal"

// Receipts are cash movement, not earned revenue. Expiry revenue is unallocated
// to channels/models; filtered consumption reports therefore omit these totals.
type OperationsFundingTotals struct {
	ReceiptsKnownUSD float64 `json:"receipts_known_usd"`
	ExpiryKnownUSD   float64 `json:"expiry_known_usd"`
	UnknownEntries   int     `json:"unknown_entries"`
	Truncated        bool    `json:"truncated"`
}

func enrichOperationsFundingTotals(report *OperationsReport, params OperationsReportParams) error {
	if DB == nil || params.ChannelID != 0 || params.ModelName != "" || params.Group != "" || params.Username != "" {
		return nil
	}
	var entries []OperationsEntry
	if err := DB.Where("created_at >= ? AND created_at <= ? AND kind IN ?", params.StartTimestamp, params.EndTimestamp, []string{"receipt", "expiry"}).Order("created_at ASC, event_key ASC").Limit(operationsReportMaxLogRows + 1).Find(&entries).Error; err != nil {
		return err
	}
	totals := &OperationsFundingTotals{}
	if len(entries) > operationsReportMaxLogRows {
		totals.Truncated = true
		entries = entries[:operationsReportMaxLogRows]
	}
	receipts, expiry := decimal.Zero, decimal.Zero
	for _, e := range entries {
		value, err := decimal.NewFromString(e.ValueUSD)
		if err != nil || value.IsNegative() {
			totals.UnknownEntries++
			continue
		}
		if e.UnknownQuota != "0" {
			totals.UnknownEntries++
		}
		if e.Kind == "receipt" {
			receipts = receipts.Add(value)
		} else {
			expiry = expiry.Add(value)
		}
	}
	totals.ReceiptsKnownUSD, _ = receipts.Float64()
	totals.ExpiryKnownUSD, _ = expiry.Float64()
	report.Funding = totals
	return nil
}
