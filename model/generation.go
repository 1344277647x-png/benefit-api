package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/creation_setting"

	"gorm.io/gorm"
)

type GenerationJobStatus string

const (
	GenerationJobPending       GenerationJobStatus = "pending"
	GenerationJobQueued        GenerationJobStatus = "queued"
	GenerationJobProcessing    GenerationJobStatus = "processing"
	GenerationJobArchiving     GenerationJobStatus = "archiving"
	GenerationJobArchiveFailed GenerationJobStatus = "archive_failed"
	GenerationJobSucceeded     GenerationJobStatus = "succeeded"
	GenerationJobFailed        GenerationJobStatus = "failed"
)

const (
	GenerationKindImage = "image"
	GenerationKindVideo = "video"
)

const (
	GenerationAssetStatusReady    = "ready"
	GenerationAssetStatusDeleting = "deleting"
)

type GenerationJob struct {
	ID              int64               `json:"-" gorm:"primaryKey"`
	PublicID        string              `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID          int                 `json:"-" gorm:"index"`
	Kind            string              `json:"kind" gorm:"type:varchar(16);index"`
	Protocol        string              `json:"protocol" gorm:"type:varchar(32)"`
	Model           string              `json:"model" gorm:"type:varchar(191);index"`
	Prompt          string              `json:"prompt" gorm:"type:text"`
	Parameters      string              `json:"parameters" gorm:"type:text"`
	TaskID          string              `json:"task_id,omitempty" gorm:"type:varchar(191);index"`
	Status          GenerationJobStatus `json:"status" gorm:"type:varchar(32);index"`
	ErrorCode       string              `json:"error_code,omitempty" gorm:"type:varchar(64)"`
	ErrorMessage    string              `json:"error_message,omitempty" gorm:"type:text"`
	ReservedBytes   int64               `json:"-" gorm:"default:0"`
	ArchiveAttempts int                 `json:"archive_attempts,omitempty" gorm:"default:0"`
	NextArchiveAt   int64               `json:"-" gorm:"index"`
	CreatedAt       int64               `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt       int64               `json:"updated_at" gorm:"autoUpdateTime"`
	ExpiresAt       int64               `json:"expires_at" gorm:"index"`
	Assets          []GenerationAsset   `json:"assets,omitempty" gorm:"-"`
	RequestedCount  int                 `json:"requested_count,omitempty" gorm:"-"`
	ResultCount     int                 `json:"result_count,omitempty" gorm:"-"`
}

type GenerationAsset struct {
	ID           int64  `json:"-" gorm:"primaryKey"`
	PublicID     string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	JobID        int64  `json:"-" gorm:"index"`
	UserID       int    `json:"-" gorm:"index"`
	Role         string `json:"role" gorm:"type:varchar(16);index"`
	RelativePath string `json:"-" gorm:"type:varchar(512);uniqueIndex"`
	MimeType     string `json:"mime_type" gorm:"type:varchar(64)"`
	SizeBytes    int64  `json:"size_bytes"`
	Status       string `json:"status" gorm:"type:varchar(24);index"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime;index"`
	ExpiresAt    int64  `json:"expires_at" gorm:"index"`
	ContentURL   string `json:"content_url,omitempty" gorm:"-"`
}

type GenerationStorageUsage struct {
	UserBytes     int64 `json:"user_bytes"`
	SystemBytes   int64 `json:"system_bytes"`
	UserLimit     int64 `json:"user_limit"`
	SystemLimit   int64 `json:"system_limit"`
	ReservedBytes int64 `json:"reserved_bytes"`
}

var generationQuotaMu sync.Mutex

func generationPublicID(prefix string) (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return prefix + key, nil
}

func generationExpiry() int64 {
	return time.Now().Add(time.Duration(creation_setting.GetSetting().RetentionDays) * 24 * time.Hour).Unix()
}

func generationStorageUsage(tx *gorm.DB, userID int) (GenerationStorageUsage, error) {
	var usage GenerationStorageUsage
	if err := tx.Model(&GenerationAsset{}).
		Select("COALESCE(SUM(size_bytes), 0)").
		Where("status = ?", "ready").
		Scan(&usage.SystemBytes).Error; err != nil {
		return usage, err
	}
	if err := tx.Model(&GenerationAsset{}).
		Select("COALESCE(SUM(size_bytes), 0)").
		Where("user_id = ? AND status = ?", userID, "ready").
		Scan(&usage.UserBytes).Error; err != nil {
		return usage, err
	}
	if err := tx.Model(&GenerationJob{}).
		Select("COALESCE(SUM(reserved_bytes), 0)").
		Where("reserved_bytes > 0").
		Scan(&usage.ReservedBytes).Error; err != nil {
		return usage, err
	}
	var userReserved int64
	if err := tx.Model(&GenerationJob{}).
		Select("COALESCE(SUM(reserved_bytes), 0)").
		Where("user_id = ? AND reserved_bytes > 0", userID).
		Scan(&userReserved).Error; err != nil {
		return usage, err
	}
	setting := creation_setting.GetSetting()
	usage.UserLimit = int64(setting.MaxUserStorageMB) * 1024 * 1024
	usage.SystemLimit = int64(setting.MaxSystemStorageMB) * 1024 * 1024
	usage.UserBytes += userReserved
	usage.SystemBytes += usage.ReservedBytes
	return usage, nil
}

func GetGenerationStorageUsage(userID int) (GenerationStorageUsage, error) {
	generationQuotaMu.Lock()
	defer generationQuotaMu.Unlock()
	return generationStorageUsage(DB, userID)
}

func ReserveGenerationJob(job *GenerationJob, bytes int64) error {
	if job == nil || job.UserID <= 0 || bytes <= 0 {
		return errors.New("invalid generation reservation")
	}
	generationQuotaMu.Lock()
	defer generationQuotaMu.Unlock()

	return DB.Transaction(func(tx *gorm.DB) error {
		return reserveGenerationJob(tx, job, bytes)
	})
}

func ReserveVideoGenerationJob(job *GenerationJob, bytes int64, maxPending int64) error {
	if job == nil || job.UserID <= 0 || bytes <= 0 {
		return errors.New("invalid generation reservation")
	}
	if maxPending <= 0 {
		maxPending = 2
	}
	generationQuotaMu.Lock()
	defer generationQuotaMu.Unlock()

	return DB.Transaction(func(tx *gorm.DB) error {
		var pending int64
		if err := tx.Model(&GenerationJob{}).
			Where("user_id = ? AND kind = ? AND status IN ?", job.UserID, GenerationKindVideo, []GenerationJobStatus{
				GenerationJobPending,
				GenerationJobQueued,
				GenerationJobProcessing,
				GenerationJobArchiving,
			}).Count(&pending).Error; err != nil {
			return err
		}
		if pending >= maxPending {
			return fmt.Errorf("too many unfinished video jobs")
		}
		return reserveGenerationJob(tx, job, bytes)
	})
}

func reserveGenerationJob(tx *gorm.DB, job *GenerationJob, bytes int64) error {
	usage, err := generationStorageUsage(tx, job.UserID)
	if err != nil {
		return err
	}
	if usage.UserBytes+bytes > usage.UserLimit {
		return fmt.Errorf("user generation storage limit exceeded")
	}
	if usage.SystemBytes+bytes > usage.SystemLimit {
		return fmt.Errorf("system generation storage limit exceeded")
	}
	publicID, err := generationPublicID("gen_")
	if err != nil {
		return err
	}
	job.PublicID = publicID
	job.ReservedBytes = bytes
	job.Status = GenerationJobPending
	job.ExpiresAt = generationExpiry()
	return tx.Create(job).Error
}

func CountPendingVideoGenerationJobs(userID int) (int64, error) {
	var count int64
	err := DB.Model(&GenerationJob{}).
		Where("user_id = ? AND kind = ? AND status IN ?", userID, GenerationKindVideo, []GenerationJobStatus{
			GenerationJobPending,
			GenerationJobQueued,
			GenerationJobProcessing,
			GenerationJobArchiving,
		}).Count(&count).Error
	return count, err
}

func InsertGenerationAsset(asset *GenerationAsset, consumeReservation bool) error {
	if asset == nil || asset.UserID <= 0 || asset.SizeBytes <= 0 || asset.RelativePath == "" {
		return errors.New("invalid generation asset")
	}
	generationQuotaMu.Lock()
	defer generationQuotaMu.Unlock()

	return DB.Transaction(func(tx *gorm.DB) error {
		usage, err := generationStorageUsage(tx, asset.UserID)
		if err != nil {
			return err
		}
		if !consumeReservation {
			if usage.UserBytes+asset.SizeBytes > usage.UserLimit {
				return fmt.Errorf("user generation storage limit exceeded")
			}
			if usage.SystemBytes+asset.SizeBytes > usage.SystemLimit {
				return fmt.Errorf("system generation storage limit exceeded")
			}
		}
		publicID, err := generationPublicID("asset_")
		if err != nil {
			return err
		}
		asset.PublicID = publicID
		asset.Status = "ready"
		asset.ExpiresAt = generationExpiry()
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		if consumeReservation && asset.JobID > 0 {
			var job GenerationJob
			if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", asset.JobID, asset.UserID).First(&job).Error; err != nil {
				return err
			}
			remaining := job.ReservedBytes - asset.SizeBytes
			if remaining < 0 {
				return fmt.Errorf("generation asset exceeds reserved storage")
			}
			if err := tx.Model(&job).Update("reserved_bytes", remaining).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func AttachGenerationInputAsset(userID int, publicID string, jobID int64) (*GenerationAsset, error) {
	assets, err := AttachGenerationInputAssets(userID, []string{publicID}, jobID)
	if err != nil {
		return nil, err
	}
	if len(assets) != 1 {
		return nil, errors.New("reference asset not found")
	}
	return &assets[0], nil
}

func AttachGenerationInputAssets(userID int, publicIDs []string, jobID int64) ([]GenerationAsset, error) {
	if userID <= 0 || jobID <= 0 || len(publicIDs) == 0 {
		return nil, errors.New("invalid reference assets")
	}
	uniqueIDs := make([]string, 0, len(publicIDs))
	seen := make(map[string]struct{}, len(publicIDs))
	for _, rawID := range publicIDs {
		publicID := strings.TrimSpace(rawID)
		if publicID == "" {
			return nil, errors.New("reference asset ids cannot be empty")
		}
		if _, exists := seen[publicID]; exists {
			continue
		}
		seen[publicID] = struct{}{}
		uniqueIDs = append(uniqueIDs, publicID)
	}
	assets := make([]GenerationAsset, 0, len(uniqueIDs))
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Lock all rows in a stable database order before reconstructing the
		// client order. This avoids deadlocks when two concurrent requests share
		// multiple reference assets but submit them in opposite orders.
		var found []GenerationAsset
		if err := lockForUpdate(tx).
			Where("public_id IN ? AND user_id = ? AND role = ? AND status = ?", uniqueIDs, userID, "input", GenerationAssetStatusReady).
			Order("id ASC").Find(&found).Error; err != nil {
			return err
		}
		if len(found) != len(uniqueIDs) {
			return errors.New("reference asset not found")
		}
		byPublicID := make(map[string]GenerationAsset, len(found))
		for _, asset := range found {
			byPublicID[asset.PublicID] = asset
		}
		for _, publicID := range uniqueIDs {
			asset, ok := byPublicID[publicID]
			if !ok {
				return errors.New("reference asset not found")
			}
			if asset.ExpiresAt > 0 && asset.ExpiresAt <= time.Now().Unix() {
				return errors.New("reference asset has expired")
			}
			if asset.JobID != 0 && asset.JobID != jobID {
				return errors.New("reference asset is already attached to another job")
			}
			assets = append(assets, asset)
		}
		idsToAttach := make([]int64, 0, len(assets))
		for _, asset := range assets {
			if asset.JobID == 0 {
				idsToAttach = append(idsToAttach, asset.ID)
			}
		}
		if len(idsToAttach) == 0 {
			return nil
		}
		result := tx.Model(&GenerationAsset{}).
			Where("id IN ? AND user_id = ? AND role = ? AND status = ? AND job_id = 0", idsToAttach, userID, "input", GenerationAssetStatusReady).
			Update("job_id", jobID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(idsToAttach)) {
			return errors.New("reference asset changed while attaching")
		}
		for i := range assets {
			if assets[i].JobID == 0 {
				assets[i].JobID = jobID
			}
		}
		return nil
	})
	return assets, err
}

func ClaimUnattachedGenerationInputAssets(userID int, publicIDs []string) ([]GenerationAsset, error) {
	if userID <= 0 || len(publicIDs) == 0 {
		return []GenerationAsset{}, nil
	}
	uniqueIDs := make([]string, 0, len(publicIDs))
	seen := make(map[string]struct{}, len(publicIDs))
	for _, publicID := range publicIDs {
		publicID = strings.TrimSpace(publicID)
		if publicID == "" {
			continue
		}
		if _, ok := seen[publicID]; ok {
			continue
		}
		seen[publicID] = struct{}{}
		uniqueIDs = append(uniqueIDs, publicID)
	}
	if len(uniqueIDs) == 0 {
		return []GenerationAsset{}, nil
	}
	var assets []GenerationAsset
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).
			Where("public_id IN ? AND user_id = ? AND role = ? AND status = ? AND job_id = 0", uniqueIDs, userID, "input", GenerationAssetStatusReady).
			Find(&assets).Error; err != nil {
			return err
		}
		if len(assets) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(assets))
		for _, asset := range assets {
			ids = append(ids, asset.ID)
		}
		result := tx.Model(&GenerationAsset{}).
			Where("id IN ? AND user_id = ? AND role = ? AND status = ? AND job_id = 0", ids, userID, "input", GenerationAssetStatusReady).
			Updates(map[string]any{
				"status":     GenerationAssetStatusDeleting,
				"expires_at": time.Now().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(assets)) {
			return errors.New("reference asset changed while preparing cleanup")
		}
		return nil
	})
	return assets, err
}

// ClaimGenerationInputAssetsForCleanup marks input assets attached to a failed
// generation job for deletion. The claim is transactional so a concurrent
// cleanup or job deletion cannot process the same ready asset twice.
func ClaimGenerationInputAssetsForCleanup(userID int, jobID int64) ([]GenerationAsset, error) {
	if userID <= 0 || jobID <= 0 {
		return []GenerationAsset{}, nil
	}
	var assets []GenerationAsset
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).
			Where("job_id = ? AND user_id = ? AND role = ? AND status = ?", jobID, userID, "input", GenerationAssetStatusReady).
			Find(&assets).Error; err != nil {
			return err
		}
		if len(assets) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(assets))
		for _, asset := range assets {
			ids = append(ids, asset.ID)
		}
		result := tx.Model(&GenerationAsset{}).
			Where("id IN ? AND job_id = ? AND user_id = ? AND role = ? AND status = ?", ids, jobID, userID, "input", GenerationAssetStatusReady).
			Updates(map[string]any{
				"status":     GenerationAssetStatusDeleting,
				"expires_at": time.Now().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(assets)) {
			return errors.New("reference asset changed while preparing cleanup")
		}
		return nil
	})
	return assets, err
}

func RestoreClaimedGenerationInputAsset(userID int, assetID int64, expiresAt int64) error {
	if userID <= 0 || assetID <= 0 {
		return nil
	}
	return DB.Model(&GenerationAsset{}).
		Where("id = ? AND user_id = ? AND role = ? AND status = ? AND job_id = 0", assetID, userID, "input", GenerationAssetStatusDeleting).
		Updates(map[string]any{
			"status":     GenerationAssetStatusReady,
			"expires_at": expiresAt,
		}).Error
}

func RestoreClaimedGenerationInputAssetForJob(userID int, jobID int64, assetID int64, expiresAt int64) error {
	if userID <= 0 || jobID <= 0 || assetID <= 0 {
		return nil
	}
	return DB.Model(&GenerationAsset{}).
		Where("id = ? AND user_id = ? AND job_id = ? AND role = ? AND status = ?", assetID, userID, jobID, "input", GenerationAssetStatusDeleting).
		Updates(map[string]any{
			"status":     GenerationAssetStatusReady,
			"expires_at": expiresAt,
		}).Error
}

func DeleteClaimedGenerationInputAssetRecord(userID int, assetID int64) error {
	if userID <= 0 || assetID <= 0 {
		return nil
	}
	return DB.Where("id = ? AND user_id = ? AND role = ? AND status = ? AND job_id = 0", assetID, userID, "input", GenerationAssetStatusDeleting).
		Delete(&GenerationAsset{}).Error
}

func DeleteClaimedGenerationInputAssetRecordForJob(userID int, jobID int64, assetID int64) error {
	if userID <= 0 || jobID <= 0 || assetID <= 0 {
		return nil
	}
	return DB.Where("id = ? AND user_id = ? AND job_id = ? AND role = ? AND status = ?", assetID, userID, jobID, "input", GenerationAssetStatusDeleting).
		Delete(&GenerationAsset{}).Error
}

func GetGenerationAssetForUser(userID int, publicID string) (*GenerationAsset, error) {
	var asset GenerationAsset
	err := DB.Where("public_id = ? AND user_id = ? AND status = ?", publicID, userID, "ready").First(&asset).Error
	return &asset, err
}

func GetGenerationJobForUser(userID int, publicID string) (*GenerationJob, error) {
	var job GenerationJob
	if err := DB.Where("public_id = ? AND user_id = ?", publicID, userID).First(&job).Error; err != nil {
		return nil, err
	}
	assets, err := GetGenerationAssetsByJobIDs([]int64{job.ID})
	if err != nil {
		return nil, err
	}
	job.Assets = assets[job.ID]
	return &job, nil
}

func GetGenerationJobByTaskID(taskID string) (*GenerationJob, error) {
	var job GenerationJob
	err := DB.Where("task_id = ?", taskID).First(&job).Error
	return &job, err
}

func ListGenerationJobs(userID int, startIdx int, num int) ([]*GenerationJob, int64, error) {
	var jobs []*GenerationJob
	var total int64
	query := DB.Model(&GenerationJob{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id DESC").Offset(startIdx).Limit(num).Find(&jobs).Error; err != nil {
		return nil, 0, err
	}
	jobIDs := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}
	assets, err := GetGenerationAssetsByJobIDs(jobIDs)
	if err != nil {
		return nil, 0, err
	}
	for _, job := range jobs {
		job.Assets = assets[job.ID]
	}
	return jobs, total, nil
}

func GetGenerationAssetsByJobIDs(jobIDs []int64) (map[int64][]GenerationAsset, error) {
	result := make(map[int64][]GenerationAsset)
	if len(jobIDs) == 0 {
		return result, nil
	}
	var assets []GenerationAsset
	if err := DB.Where("job_id IN ? AND status = ?", jobIDs, "ready").Order("id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	for _, asset := range assets {
		result[asset.JobID] = append(result[asset.JobID], asset)
	}
	return result, nil
}

func UpdateGenerationJob(jobID int64, updates map[string]any) error {
	if jobID <= 0 || len(updates) == 0 {
		return nil
	}
	return DB.Model(&GenerationJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func FinishGenerationJob(jobID int64, status GenerationJobStatus, errorCode string, errorMessage string) error {
	return UpdateGenerationJob(jobID, map[string]any{
		"status":          status,
		"error_code":      errorCode,
		"error_message":   errorMessage,
		"reserved_bytes":  0,
		"next_archive_at": 0,
	})
}

func FindGenerationVideoJobsForArchival(now int64, limit int) ([]*GenerationJob, error) {
	if limit <= 0 {
		limit = 50
	}
	var jobs []*GenerationJob
	err := DB.Where("kind = ? AND status IN ? AND (next_archive_at = 0 OR next_archive_at <= ?)", GenerationKindVideo, []GenerationJobStatus{
		GenerationJobQueued,
		GenerationJobProcessing,
		GenerationJobArchiving,
	}, now).Order("id ASC").Limit(limit).Find(&jobs).Error
	return jobs, err
}

func ClaimGenerationJobArchival(jobID int64, now int64) (bool, error) {
	if jobID <= 0 {
		return false, nil
	}
	result := DB.Model(&GenerationJob{}).
		Where("id = ? AND status IN ? AND (next_archive_at = 0 OR next_archive_at <= ?)", jobID, []GenerationJobStatus{
			GenerationJobQueued,
			GenerationJobProcessing,
			GenerationJobArchiving,
		}, now).
		Updates(map[string]any{
			"status":          GenerationJobArchiving,
			"next_archive_at": now + 300,
		})
	return result.RowsAffected > 0, result.Error
}

func ListExpiredGenerationAssets(now int64, limit int) ([]GenerationAsset, error) {
	if limit <= 0 {
		limit = 100
	}
	var assets []GenerationAsset
	err := DB.Where("expires_at > 0 AND expires_at <= ?", now).Order("id ASC").Limit(limit).Find(&assets).Error
	return assets, err
}

func DeleteGenerationAssetRecord(id int64) error {
	if id <= 0 {
		return nil
	}
	return DB.Delete(&GenerationAsset{}, id).Error
}

func DeleteGenerationJobRecord(id int64) error {
	if id <= 0 {
		return nil
	}
	return DB.Delete(&GenerationJob{}, id).Error
}

func DeleteExpiredEmptyGenerationJobs(now int64, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var jobs []GenerationJob
	if err := DB.Where("expires_at > 0 AND expires_at <= ?", now).Order("id ASC").Limit(limit).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		var count int64
		if err := DB.Model(&GenerationAsset{}).Where("job_id = ?", job.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := DeleteGenerationJobRecord(job.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func DeleteGenerationJobForUser(userID int, publicID string) (*GenerationJob, []GenerationAsset, error) {
	job, err := GetGenerationJobForUser(userID, publicID)
	if err != nil {
		return nil, nil, err
	}
	if job.Status == GenerationJobPending || job.Status == GenerationJobQueued || job.Status == GenerationJobProcessing || job.Status == GenerationJobArchiving {
		return nil, nil, errors.New("generation job is still running")
	}
	return job, job.Assets, nil
}

func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
