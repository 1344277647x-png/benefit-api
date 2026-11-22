package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MoeForge/moego/ojson"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/channel_health_setting"
	"github.com/QuantumNous/new-api/setting/creation_setting"
)

const creationArchiveMaxAttempts = 3

func StartGenerationMaintenance() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		archiveTicker := time.NewTicker(15 * time.Second)
		cleanupTicker := time.NewTicker(time.Hour)
		assetTicker := time.NewTicker(time.Minute)
		defer archiveTicker.Stop()
		defer cleanupTicker.Stop()
		defer assetTicker.Stop()
		for {
			func() {
				defer func() {
					if recover() != nil {
						common.SysError("generation maintenance recovered from panic")
					}
				}()
				select {
				case <-assetTicker.C:
					if _, err := model.DeleteExpiredContentAudits(time.Now().Unix()); err != nil {
						common.SysError("cleanup content audits failed")
					}
					if _, err := service.CleanupExpiredGenerationAssets(time.Now().Unix(), 1000); err != nil {
						common.SysError("cleanup generation assets: " + err.Error())
					}
				case <-archiveTicker.C:
					if creation_setting.Enabled() {
						if err := processGenerationVideoArchives(context.Background(), time.Now()); err != nil {
							common.SysError("process generation video archives: " + err.Error())
						}
					}
				case <-cleanupTicker.C:
					if _, err := model.FinalizeDueTeamDissolutions(100); err != nil {
						common.SysError("finalize team dissolutions: " + err.Error())
					}
					healthSetting := channel_health_setting.GetSetting()
					cutoff := time.Now().Add(-time.Duration(healthSetting.RetentionDays) * 24 * time.Hour).Unix()
					if err := model.DeleteChannelHealthBucketsBefore(cutoff); err != nil {
						common.SysError("cleanup channel health buckets: " + err.Error())
					}
				}
			}()
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
		finishErr := model.FinishGenerationJob(job.ID, model.GenerationJobFailed, "upstream_task_failed", message)
		// The input reference is only needed while the upstream task is active.
		// Once the task has failed, release it so a failed video cannot consume
		// the user's media quota until retention expiry.
		cleanupCreationJobInputAssets(job)
		return finishErr
	case model.TaskStatusSuccess:
		assetsByJob, err := model.GetGenerationAssetsByJobIDs([]int64{job.ID})
		if err != nil {
			return err
		}
		for _, asset := range assetsByJob[job.ID] {
			if asset.Role == "output" && asset.Status == "ready" {
				if err := model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", ""); err != nil {
					return err
				}
				service.RecordContentAudit(service.ContentAuditWrite{UserID: job.UserID, ModelName: job.Model,
					RequestID: job.PublicID, Source: "creation_video", Status: "succeeded", Input: job.Prompt,
					Output: job.Parameters, ResultReferences: asset.PublicID})
				return nil
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
		if err := model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", ""); err != nil {
			return err
		}
		service.RecordContentAudit(service.ContentAuditWrite{UserID: job.UserID, ModelName: job.Model,
			RequestID: job.PublicID, Source: "creation_video", Status: "succeeded", Input: job.Prompt,
			Output: job.Parameters, ResultReferences: asset.PublicID})
		return nil
	default:
		return model.UpdateGenerationJob(job.ID, map[string]any{
			"status":          model.GenerationJobProcessing,
			"next_archive_at": now.Add(15 * time.Second).Unix(),
		})
	}
}

func archiveCompletedVideoTask(parent context.Context, job *model.GenerationJob, task *model.Task) (*model.GenerationAsset, error) {
	if !task.ResultRetrievable() {
		return nil, errors.New("video result is not available")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	descriptor, err := resolveTaskPluginVideo(ctx, task, relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if descriptor == nil {
		channel, err := model.CacheGetChannel(task.ChannelId)
		if err != nil {
			return nil, fmt.Errorf("load video channel: %w", err)
		}
		descriptor = legacyCreationVideoContent(task, channel)
	}

	request := service.GenerationAssetSaveRequest{
		UserID:             job.UserID,
		JobID:              job.ID,
		Role:               "output",
		Kind:               model.GenerationKindVideo,
		MaxBytes:           service.GenerationAssetLimit(model.GenerationKindVideo),
		ConsumeReservation: true,
	}
	if strings.HasPrefix(descriptor.URL, "data:") {
		data, err := decodeGenerationVideoDataURL(descriptor.URL, request.MaxBytes)
		if err != nil {
			return nil, err
		}
		request.Reader = bytes.NewReader(data)
		return service.SaveGenerationAsset(request)
	}
	resp, err := openTaskMedia(ctx, nil, task, descriptor)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("video upstream returned status %d", resp.StatusCode)
	}
	if len(descriptor.ContentPath) != 0 {
		encoded, err := readTaskMediaContentPath(resp.Body, descriptor, request.MaxBytes)
		if err != nil {
			return nil, err
		}
		data, err := decodeGenerationVideoBase64(encoded, request.MaxBytes)
		if err != nil {
			return nil, err
		}
		request.Reader = bytes.NewReader(data)
	} else {
		if resp.ContentLength > request.MaxBytes {
			return nil, service.ErrGenerationAssetTooLarge
		}
		request.Reader = resp.Body
	}
	return service.SaveGenerationAsset(request)
}

func legacyCreationVideoContent(task *model.Task, channel *model.Channel) *relaychannel.TaskContentRequest {
	videoURL := strings.TrimSpace(task.GetResultURL())
	if isTaskMediaFallbackLoop(videoURL, task.TaskID) {
		videoURL = ""
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channel.Type)
	}
	key := strings.TrimSpace(task.PrivateData.Key)
	if key == "" {
		key = strings.TrimSpace(channel.Key)
	}
	descriptor := &relaychannel.TaskContentRequest{Method: http.MethodGet, Credentialless: true}
	switch channel.Type {
	case constant.ChannelTypeGemini:
		// Legacy tasks can persist only the operation reply, with a local
		// proxy URL in ResultURL.
		for _, path := range []string{"uri", "response.generateVideoResponse.generatedSamples.0.video.uri", "response.videos.0.uri", "response.video", "response.uri"} {
			if candidate := ojson.GetBytes(task.Data, path).Str; candidate != "" {
				videoURL = candidate
				break
			}
		}
		target, targetErr := url.Parse(videoURL)
		origin, originErr := url.Parse(baseURL)
		if key != "" && targetErr == nil && originErr == nil &&
			target.Scheme == "https" && sameTaskMediaOrigin(origin, target) {
			descriptor.Headers = map[string]string{"x-goog-api-key": key}
			descriptor.Credentialless = false
		}
	case constant.ChannelTypeVertexAi:
		if videoURL == "" {
			for _, path := range []string{"response.videos.0.bytesBase64Encoded", "response.bytesBase64Encoded", "response.video"} {
				if candidate := strings.TrimSpace(ojson.GetBytes(task.Data, path).Str); candidate != "" {
					videoURL = candidate
					if !strings.HasPrefix(candidate, "data:") && !strings.Contains(candidate, "://") {
						videoURL = "data:video/mp4;base64," + candidate
					}
					break
				}
			}
		}
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		if !taskHasPluginExecution(task) && baseURL != "" && task.GetUpstreamTaskID() != "" {
			videoURL = fmt.Sprintf("%s/v1/videos/%s/content", strings.TrimRight(baseURL, "/"), url.PathEscape(task.GetUpstreamTaskID()))
			if key != "" {
				descriptor.Headers = map[string]string{"Authorization": "Bearer " + key}
				descriptor.Credentialless = false
			}
		}
	}
	descriptor.URL = videoURL
	return descriptor
}

func decodeGenerationVideoDataURL(dataURL string, maxBytes int64) ([]byte, error) {
	comma := strings.Index(dataURL, ",")
	if comma < 0 || !strings.Contains(dataURL[:comma], ";base64") {
		return nil, errors.New("unsupported video data URL")
	}
	return decodeGenerationVideoBase64(dataURL[comma+1:], maxBytes)
}

func decodeGenerationVideoBase64(encoded string, maxBytes int64) ([]byte, error) {
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
