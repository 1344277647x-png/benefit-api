package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const teamTokenPrefix = "tmb_" // underscore is absent from personal token alphabet

func CreateTeamToken(userId int, name, group, modelLimits string) (*Token, error) {
	name = strings.TrimSpace(name)
	if userId <= 0 || name == "" || len([]rune(name)) > 50 || len(modelLimits) > 4096 {
		return nil, errors.New("invalid team token")
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	token := &Token{UserId: userId, Name: name, Key: teamTokenPrefix + key,
		Status: common.TokenStatusDisabled, TeamEnabled: true, UnlimitedQuota: true,
		CreatedTime: common.GetTimestamp(), ExpiredTime: -1, Group: group,
		ModelLimitsEnabled: modelLimits != "", ModelLimits: modelLimits}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var member TeamMember
		if err := tx.Where("user_id = ?", userId).First(&member).Error; err != nil {
			return err
		}
		var team Team
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", member.TeamId, TeamStatusActive).First(&team).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		var sub TeamSubscription
		if err := tx.Where("team_id = ? AND status = ? AND start_time <= ? AND end_time > ?", team.Id, TeamStatusActive, now, now).First(&sub).Error; err != nil {
			return errors.New("active team subscription required")
		}
		token.TeamId = team.Id
		return tx.Create(token).Error
	})
	return token, err
}

// ValidateTeamToken deliberately bypasses the Redis token snapshot: membership,
// subscription and the dedicated enabled bit are checked against the database
// on every request. The legacy status remains disabled for rollback safety.
func ValidateTeamToken(key string) (*Token, error) {
	if !strings.HasPrefix(key, teamTokenPrefix) {
		return nil, ErrTokenInvalid
	}
	var token Token
	if err := DB.Where(commonKeyCol+" = ? AND team_id > 0", key).First(&token).Error; err != nil {
		return nil, ErrTokenInvalid
	}
	if token.Status != common.TokenStatusDisabled || !token.TeamEnabled ||
		(token.ExpiredTime != -1 && token.ExpiredTime <= common.GetTimestamp()) {
		return nil, ErrTokenInvalid
	}
	var member TeamMember
	if err := DB.Where("team_id = ? AND user_id = ?", token.TeamId, token.UserId).First(&member).Error; err != nil {
		return nil, ErrTokenInvalid
	}
	var team Team
	if err := DB.Where("id = ? AND status = ?", token.TeamId, TeamStatusActive).First(&team).Error; err != nil {
		return nil, ErrTokenInvalid
	}
	now := common.GetTimestamp()
	var sub TeamSubscription
	if err := DB.Where("team_id = ? AND status = ? AND start_time <= ? AND end_time > ?", token.TeamId, TeamStatusActive, now, now).First(&sub).Error; err != nil {
		return nil, ErrTokenInvalid
	}
	return &token, nil
}

func ListTeamTokens(userId, teamId int) ([]Token, error) {
	var tokens []Token
	err := DB.Where("user_id = ? AND team_id = ?", userId, teamId).Order("id desc").Find(&tokens).Error
	for i := range tokens {
		tokens[i].Clean()
	}
	return tokens, err
}

func DisableTeamToken(userId, teamId, tokenId int) error {
	var token Token
	if err := DB.Where("id = ? AND user_id = ? AND team_id = ?", tokenId, userId, teamId).First(&token).Error; err != nil {
		return err
	}
	result := DB.Model(&token).Where("team_enabled = ?", true).Update("team_enabled", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTokenInvalid
	}
	return nil
}
