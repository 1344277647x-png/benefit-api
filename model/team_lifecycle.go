package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func PlanTeamDissolution(ownerID int) (int64, error) {
	var dissolveAt int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("owner_id = ? AND status = ?", ownerID, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		if team.DissolveAt > 0 {
			dissolveAt = team.DissolveAt
			return nil
		}
		var blockers int64
		if err := tx.Model(&TeamOrder{}).Where("team_id = ? AND status IN ?", team.Id,
			[]string{common.TopUpStatusPending, TeamOrderStatusPaidAfterCancel}).Count(&blockers).Error; err != nil {
			return err
		}
		if blockers > 0 {
			return errors.New("pending or late-paid team orders must be resolved first")
		}
		if err := tx.Model(&TeamSubscription{}).Where("team_id = ? AND status = ?", team.Id, TeamStatusActive).
			Select("COALESCE(MAX(end_time), 0)").Scan(&dissolveAt).Error; err != nil {
			return err
		}
		if dissolveAt <= common.GetTimestamp() {
			return errors.New("team has no paid term to finish")
		}
		if err := tx.Model(&team).Update("dissolve_at", dissolveAt).Error; err != nil {
			return err
		}
		return enqueueBusinessEventTx(tx, &BusinessEvent{EventKey: fmt.Sprintf("team:dissolution:planned:%d:%d", team.Id, dissolveAt),
			Category: BusinessEventTeam, Action: "team.dissolution_planned", UserId: ownerID,
			Content: "Team dissolution scheduled", CreatedAt: common.GetTimestamp()}, map[string]any{"team_id": team.Id, "dissolve_at": dissolveAt})
	})
	return dissolveAt, err
}

func RevokeTeamDissolution(ownerID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var team Team
		if err := lockForUpdate(tx).Where("owner_id = ? AND status = ?", ownerID, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		if team.DissolveAt == 0 || team.DissolveAt <= common.GetTimestamp() {
			return errors.New("team has no revocable dissolution plan")
		}
		old := team.DissolveAt
		if err := tx.Model(&team).Update("dissolve_at", 0).Error; err != nil {
			return err
		}
		return enqueueBusinessEventTx(tx, &BusinessEvent{EventKey: fmt.Sprintf("team:dissolution:revoked:%d:%d", team.Id, old),
			Category: BusinessEventTeam, Action: "team.dissolution_revoked", UserId: ownerID,
			Content: "Team dissolution revoked", CreatedAt: common.GetTimestamp()}, map[string]any{"team_id": team.Id, "previous_dissolve_at": old})
	})
}

func FinalizeDueTeamDissolutions(limit int) (int, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	now := GetDBTimestamp()
	var teams []Team
	if err := DB.Where("status = ? AND dissolve_at > 0 AND dissolve_at <= ?", TeamStatusActive, now).Order("id asc").Limit(limit).Find(&teams).Error; err != nil {
		return 0, err
	}
	completed := 0
	for _, candidate := range teams {
		err := DB.Transaction(func(tx *gorm.DB) error {
			var team Team
			if err := lockForUpdate(tx).Where("id = ? AND status = ? AND dissolve_at > 0 AND dissolve_at <= ?", candidate.Id, TeamStatusActive, now).First(&team).Error; err != nil {
				return err
			}
			if err := tx.Model(&Token{}).Where("team_id = ? AND team_enabled = ?", team.Id, true).Update("team_enabled", false).Error; err != nil {
				return err
			}
			if err := tx.Where("team_id = ?", team.Id).Delete(&TeamMember{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&TeamInvitation{}).Where("team_id = ? AND status = ?", team.Id, TeamInvitationPending).Update("status", TeamInvitationCancelled).Error; err != nil {
				return err
			}
			if err := tx.Model(&TeamSubscription{}).Where("team_id = ? AND status = ?", team.Id, TeamStatusActive).Update("status", TeamStatusDissolved).Error; err != nil {
				return err
			}
			if err := tx.Model(&team).Updates(map[string]any{"status": TeamStatusDissolved}).Error; err != nil {
				return err
			}
			return enqueueBusinessEventTx(tx, &BusinessEvent{EventKey: fmt.Sprintf("team:dissolution:completed:%d", team.Id),
				Category: BusinessEventTeam, Action: "team.dissolution_completed", UserId: team.OwnerId,
				Content: "Team dissolved", CreatedAt: now}, map[string]any{"team_id": team.Id})
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return completed, err
		}
		completed++
	}
	return completed, nil
}
