package model

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelHealthSnapshot struct {
	ID                  int64  `json:"-" gorm:"primaryKey"`
	ChannelID           int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_health_snapshot,priority:1"`
	ModelName           string `json:"model" gorm:"type:varchar(191);uniqueIndex:idx_channel_health_snapshot,priority:2"`
	EndpointType        string `json:"endpoint_type" gorm:"type:varchar(40)"`
	LastSampleAt        int64  `json:"last_sample_at" gorm:"index"`
	LastSuccessAt       int64  `json:"last_success_at"`
	LastLatencyMs       int64  `json:"last_latency_ms"`
	LastTTFTMs          int64  `json:"last_ttft_ms"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastErrorClass      string `json:"last_error_class,omitempty" gorm:"type:varchar(40)"`
	LastErrorCode       string `json:"last_error_code,omitempty" gorm:"type:varchar(80)"`
	LastHTTPStatus      int    `json:"last_http_status,omitempty"`
	UpdatedAt           int64  `json:"-" gorm:"autoUpdateTime"`
}

type ChannelHealthBucket struct {
	ID             int64  `json:"-" gorm:"primaryKey"`
	ChannelID      int    `json:"channel_id" gorm:"uniqueIndex:idx_channel_health_bucket,priority:1"`
	ModelName      string `json:"model" gorm:"type:varchar(191);uniqueIndex:idx_channel_health_bucket,priority:2"`
	BucketTs       int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_health_bucket,priority:3;index"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	FailureCount   int64  `json:"failure_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	LatencyCount   int64  `json:"latency_count"`
	TTFTSumMs      int64  `json:"ttft_sum_ms"`
	TTFTCount      int64  `json:"ttft_count"`
}

type ChannelHealthSample struct {
	ChannelID    int
	ModelName    string
	EndpointType string
	SampledAt    int64
	Success      bool
	LatencyMs    int64
	TTFTMs       int64
	HasTTFT      bool
	ErrorClass   string
	ErrorCode    string
	HTTPStatus   int
}

func IsChannelHealthFailureStatus(status int) bool {
	return status <= 0 || status == 408 || status == 429 || status >= 500
}

func ChannelHealthEndpoint(path string) string {
	switch {
	case strings.Contains(path, "/images/"):
		return "image-generation"
	case strings.Contains(path, "/videos"):
		return "openai-video"
	case strings.Contains(path, ":generateContent"):
		return "gemini"
	case strings.Contains(path, "/responses"):
		return "openai-response"
	default:
		return "openai"
	}
}

type ChannelHealthWindow struct {
	ChannelID      int    `json:"channel_id"`
	ModelName      string `json:"model"`
	RequestCount   int64  `json:"request_count"`
	SuccessCount   int64  `json:"success_count"`
	FailureCount   int64  `json:"failure_count"`
	TotalLatencyMs int64  `json:"total_latency_ms"`
	LatencyCount   int64  `json:"latency_count"`
	TTFTSumMs      int64  `json:"ttft_sum_ms"`
	TTFTCount      int64  `json:"ttft_count"`
}

func RecordChannelHealthSample(sample ChannelHealthSample) error {
	if sample.ChannelID <= 0 || sample.ModelName == "" {
		return nil
	}
	if sample.SampledAt <= 0 {
		sample.SampledAt = time.Now().Unix()
	}
	bucketTs := sample.SampledAt - sample.SampledAt%60

	return DB.Transaction(func(tx *gorm.DB) error {
		var snapshot ChannelHealthSnapshot
		err := lockForUpdate(tx).
			Where("channel_id = ? AND model_name = ?", sample.ChannelID, sample.ModelName).
			First(&snapshot).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if snapshot.ID == 0 {
			snapshot.ChannelID = sample.ChannelID
			snapshot.ModelName = sample.ModelName
		}
		if sample.SampledAt >= snapshot.LastSampleAt {
			snapshot.EndpointType = sample.EndpointType
			snapshot.LastSampleAt = sample.SampledAt
			snapshot.LastLatencyMs = sample.LatencyMs
			if sample.HasTTFT {
				snapshot.LastTTFTMs = sample.TTFTMs
			}
			if sample.Success {
				snapshot.LastSuccessAt = sample.SampledAt
				snapshot.ConsecutiveFailures = 0
				snapshot.LastErrorClass = ""
				snapshot.LastErrorCode = ""
				snapshot.LastHTTPStatus = 0
			} else {
				snapshot.ConsecutiveFailures++
				snapshot.LastErrorClass = sample.ErrorClass
				snapshot.LastErrorCode = sample.ErrorCode
				snapshot.LastHTTPStatus = sample.HTTPStatus
			}
			if snapshot.ID == 0 {
				if err := tx.Create(&snapshot).Error; err != nil {
					return err
				}
			} else if err := tx.Save(&snapshot).Error; err != nil {
				return err
			}
		}

		bucket := ChannelHealthBucket{
			ChannelID:      sample.ChannelID,
			ModelName:      sample.ModelName,
			BucketTs:       bucketTs,
			RequestCount:   1,
			TotalLatencyMs: sample.LatencyMs,
		}
		if sample.Success {
			bucket.SuccessCount = 1
		} else {
			bucket.FailureCount = 1
		}
		if sample.HasTTFT {
			bucket.TotalLatencyMs = 0
		} else {
			bucket.LatencyCount = 1
		}
		if sample.HasTTFT {
			bucket.TTFTSumMs = sample.TTFTMs
			bucket.TTFTCount = 1
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "channel_id"}, {Name: "model_name"}, {Name: "bucket_ts"}},
			DoUpdates: clause.Assignments(map[string]any{
				"request_count":    gorm.Expr("channel_health_buckets.request_count + ?", bucket.RequestCount),
				"success_count":    gorm.Expr("channel_health_buckets.success_count + ?", bucket.SuccessCount),
				"failure_count":    gorm.Expr("channel_health_buckets.failure_count + ?", bucket.FailureCount),
				"total_latency_ms": gorm.Expr("channel_health_buckets.total_latency_ms + ?", bucket.TotalLatencyMs),
				"latency_count":    gorm.Expr("channel_health_buckets.latency_count + ?", bucket.LatencyCount),
				"ttft_sum_ms":      gorm.Expr("channel_health_buckets.ttft_sum_ms + ?", bucket.TTFTSumMs),
				"ttft_count":       gorm.Expr("channel_health_buckets.ttft_count + ?", bucket.TTFTCount),
			}),
		}).Create(&bucket).Error
	})
}

func GetChannelHealthSnapshots() ([]ChannelHealthSnapshot, error) {
	var snapshots []ChannelHealthSnapshot
	err := DB.Find(&snapshots).Error
	return snapshots, err
}

func GetEnabledChannelModelCounts() (map[string]int, error) {
	type modelCount struct {
		Model string `gorm:"column:model"`
		Count int    `gorm:"column:count"`
	}
	rows := make([]modelCount, 0)
	err := DB.Table("abilities").
		Select("abilities.model AS model, COUNT(DISTINCT abilities.channel_id) AS count").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.enabled = ? AND channels.status = ?", true, common.ChannelStatusEnabled).
		Group("abilities.model").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.Model] = row.Count
	}
	return result, nil
}

func GetChannelHealthWindows(startTs int64) ([]ChannelHealthWindow, error) {
	var windows []ChannelHealthWindow
	err := DB.Model(&ChannelHealthBucket{}).
		Select("channel_id, model_name, SUM(request_count) AS request_count, SUM(success_count) AS success_count, SUM(failure_count) AS failure_count, SUM(total_latency_ms) AS total_latency_ms, SUM(latency_count) AS latency_count, SUM(ttft_sum_ms) AS ttft_sum_ms, SUM(ttft_count) AS ttft_count").
		Where("bucket_ts >= ?", startTs).
		Group("channel_id, model_name").
		Find(&windows).Error
	return windows, err
}

func GetChannelHealthHistory(channelID int, startTs int64) ([]ChannelHealthBucket, error) {
	var buckets []ChannelHealthBucket
	err := DB.Where("channel_id = ? AND bucket_ts >= ?", channelID, startTs).
		Order("bucket_ts ASC").Find(&buckets).Error
	return buckets, err
}

func DeleteChannelHealthBucketsBefore(cutoff int64) error {
	if cutoff <= 0 {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoff).Delete(&ChannelHealthBucket{}).Error
}
