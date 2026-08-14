package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/channel_health_setting"
	"github.com/QuantumNous/new-api/setting/creation_setting"
)

const creationArchiveMaxAttempts = 3

func StartGenerationMaintenance() {
	go func() {
		archiveTicker := time.NewTicker(15 * time.Second)
		cleanupTicker := time.NewTicker(time.Hour)
		defer archiveTicker.Stop()
		defer cleanupTicker.Stop()
		for {
			select {
			case <-archiveTicker.C:
				if creation_setting.Enabled() {
					if err := processGenerationVideoArchives(context.Background(), time.Now()); err != nil {
						common.SysError("process generation video archives: " + err.Error())
					}
				}
			case <-cleanupTicker.C:
				if _, err := service.CleanupExpiredGenerationAssets(time.Now().Unix(), 100); err != nil {
					common.SysError("cleanup generation assets: " + err.Error())
				}
				healthSetting := channel_health_setting.GetSetting()
				cutoff := time.Now().Add(-time.Duration(healthSetting.RetentionDays) * 24 * time.Hour).Unix()
				if err := model.DeleteChannelHealthBucketsBefore(cutoff); err != nil {
					common.SysError("cleanup channel health buckets: " + err.Error())
				}
			}
		}
	}()
}

func processGenerationVideoArchives(ctx context.Context, now time.Time) error {
	jobs, err := model.FindGenerationVideoJobsForArchival(now.Unix(), 50)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := processGenerationVideoArchive(ctx, job, now); err != nil {
			common.SysError(fmt.Sprintf("archive generation job %s: %s", job.PublicID, err.Error()))
		}
	}
	return nil
}

func processGenerationVideoArchive(ctx context.Context, job *model.GenerationJob, now time.Time) error {
	if job == nil || job.TaskID == "" {
		return nil
	}
	task, exists, err := model.GetByTaskId(job.UserID, job.TaskID)
	if err != nil {
		return err
	}
	if !exists || task == nil {
		return updateGenerationArchiveFailure(job, now, errors.New("linked video task not found"))
	}
	switch task.Status {
	case model.TaskStatusFailure:
		message := strings.TrimSpace(task.FailReason)
		if message == "" {
			message = "video generation failed"
		}
		return model.FinishGenerationJob(job.ID, model.GenerationJobFailed, "upstream_task_failed", message)
	case model.TaskStatusSuccess:
		assetsByJob, err := model.GetGenerationAssetsByJobIDs([]int64{job.ID})
		if err != nil {
			return err
		}
		for _, asset := range assetsByJob[job.ID] {
			if asset.Role == "output" && asset.Status == "ready" {
				return model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", "")
			}
		}
		claimed, err := model.ClaimGenerationJobArchival(job.ID, now.Unix())
		if err != nil || !claimed {
			return err
		}
		asset, err := archiveCompletedVideoTask(ctx, job, task)
		if err != nil {
			return updateGenerationArchiveFailure(job, now, err)
		}
		if asset == nil {
			return updateGenerationArchiveFailure(job, now, errors.New("video archive returned no asset"))
		}
		return model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", "")
	default:
		return model.UpdateGenerationJob(job.ID, map[string]any{
			"status":          model.GenerationJobProcessing,
			"next_archive_at": now.Add(15 * time.Second).Unix(),
		})
	}
}

func archiveCompletedVideoTask(parent context.Context, job *model.GenerationJob, task *model.Task) (*model.GenerationAsset, error) {
	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("load video channel: %w", err)
	}
	videoURL := strings.TrimSpace(task.GetResultURL())
	headers := make(http.Header)
	switch channel.Type {
	case constant.ChannelTypeGemini:
		key := strings.TrimSpace(task.PrivateData.Key)
		if key == "" {
			key = strings.TrimSpace(channel.Key)
		}
		videoURL, err = getGeminiVideoURL(channel, task, key)
		if err == nil && key != "" {
			headers.Set("x-goog-api-key", key)
		}
	case constant.ChannelTypeVertexAi:
		videoURL, err = getVertexVideoURL(channel, task)
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		baseURL := constant.ChannelBaseURLs[channel.Type]
		if channel.GetBaseURL() != "" {
			baseURL = channel.GetBaseURL()
		}
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content", strings.TrimRight(baseURL, "/"), task.GetUpstreamTaskID())
		headers.Set("Authorization", "Bearer "+channel.Key)
	}
	if err != nil {
		return nil, err
	}
	if videoURL == "" {
		return nil, errors.New("video result URL is empty")
	}

	request := service.GenerationAssetSaveRequest{
		UserID:             job.UserID,
		JobID:              job.ID,
		Role:               "output",
		Kind:               model.GenerationKindVideo,
		MaxBytes:           service.GenerationAssetLimit(model.GenerationKindVideo),
		ConsumeReservation: true,
	}
	if strings.HasPrefix(videoURL, "data:") {
		data, err := decodeGenerationVideoDataURL(videoURL, request.MaxBytes)
		if err != nil {
			return nil, err
		}
		request.Reader = bytes.NewReader(data)
		return service.SaveGenerationAsset(request)
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	return service.SaveRemoteGenerationAssetWithHeaders(ctx, videoURL, headers, request)
}

func decodeGenerationVideoDataURL(dataURL string, maxBytes int64) ([]byte, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 || !strings.Contains(dataURL[:comma], ";base64") {
		return nil, errors.New("unsupported video data URL")
	}
	encoded := dataURL[comma+1:]
	if int64(len(encoded)) > maxBytes*4/3+8 {
		return nil, service.ErrGenerationAssetTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, fmt.Errorf("decode video data URL: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, service.ErrGenerationAssetTooLarge
	}
	return data, nil
}

func updateGenerationArchiveFailure(job *model.GenerationJob, now time.Time, archiveErr error) error {
	attempts := job.ArchiveAttempts + 1
	status := model.GenerationJobArchiving
	nextArchiveAt := now.Add(time.Duration(attempts) * time.Minute).Unix()
	if attempts >= creationArchiveMaxAttempts {
		status = model.GenerationJobArchiveFailed
		nextArchiveAt = 0
	}
	message := archiveErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	return model.UpdateGenerationJob(job.ID, map[string]any{
		"status":           status,
		"archive_attempts": attempts,
		"next_archive_at":  nextArchiveAt,
		"error_code":       "video_archive_failed_" + strconv.Itoa(attempts),
		"error_message":    message,
	})
}
