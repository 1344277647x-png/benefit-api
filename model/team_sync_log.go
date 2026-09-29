package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func PendingTeamSyncBillingEvents(ctx context.Context, limit int) ([]TeamSyncBillingEvent, error) {
	if limit <= 0 || limit > 100 {
		return nil, errors.New("invalid team sync billing event batch size")
	}
	var events []TeamSyncBillingEvent
	err := DB.WithContext(ctx).Where("ready = ? AND delivered_at = 0", true).
		Order("created_at asc, request_id asc").Limit(limit).Find(&events).Error
	return events, err
}

func DeliverTeamSyncBillingLog(ctx context.Context, event TeamSyncBillingEvent) error {
	if !TeamTaskLogDeliverySupported() || !event.Ready || event.RequestId == "" {
		return errors.New("transactional team sync log delivery unavailable")
	}
	var usage TeamUsage
	if err := DB.WithContext(ctx).Where("request_id = ? AND status = ?", event.RequestId, "settled").First(&usage).Error; err != nil || usage.FinalAmount != event.FinalQuota || usage.TeamId != event.TeamId || usage.MemberUserId != event.UserId {
		return fmt.Errorf("team sync billing event is not settled: %s", event.RequestId)
	}
	username, _ := GetUsernameById(event.UserId, false)
	tokenName := ""
	if event.TokenId > 0 {
		if token, err := GetTokenById(event.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	other := map[string]interface{}{}
	if event.BillingDetails != "" {
		if err := common.UnmarshalJsonStr(event.BillingDetails, &other); err != nil {
			return fmt.Errorf("invalid persisted team billing details: %w", err)
		}
	}
	other["billing_source"] = "team"
	other["team_id"] = event.TeamId
	other["member_user_id"] = event.UserId
	other["business_label"] = "team_consumption"
	other["sync_billing_event"] = true
	return LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&TeamSyncLogReceipt{
			RequestId: event.RequestId, CreatedAt: common.GetTimestamp(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Create(&Log{UserId: event.UserId, Username: username,
			CreatedAt: event.CreatedAt, Type: LogTypeConsume, TokenName: tokenName,
			ModelName: event.ModelName, Quota: int(event.FinalQuota),
			UseTime: event.UseTimeSeconds, IsStream: event.IsStream,
			PromptTokens: event.PromptTokens, CompletionTokens: event.CompletionTokens,
			ChannelId: event.ChannelId, TokenId: event.TokenId, Group: event.Group,
			RequestId: event.RequestId, Other: common.MapToJsonStr(other),
		}).Error
	})
}

func MarkTeamSyncBillingEventDelivered(ctx context.Context, requestId string) error {
	return DB.WithContext(ctx).Model(&TeamSyncBillingEvent{}).
		Where("request_id = ? AND ready = ? AND delivered_at = 0", requestId, true).
		Update("delivered_at", GetDBTimestamp()).Error
}
