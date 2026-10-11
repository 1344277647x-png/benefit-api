package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OperationsState struct {
	Id        int   `gorm:"primaryKey"`
	StartedAt int64 `gorm:"type:bigint;not null"`
}

// Insert once on the first upgraded startup; deployments and restarts preserve
// the original cutoff, including when the application is rolled back temporarily.
func EnsureOperationsStart(db *gorm.DB) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&OperationsState{Id: 1, StartedAt: common.GetTimestamp()}).Error
}

func operationsStartTimestamp() (int64, error) {
	if DB == nil || !DB.Migrator().HasTable(&OperationsState{}) {
		return 0, nil
	}
	var state OperationsState
	if err := DB.First(&state, 1).Error; err != nil {
		return 0, err
	}
	return state.StartedAt, nil
}
