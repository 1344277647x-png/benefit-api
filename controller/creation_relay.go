package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/creation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

var errCreationResponseTooLarge = errors.New("creation response exceeds the archive limit")

type creationResponseCapture struct {
	gin.ResponseWriter
	body     bytes.Buffer
	status   int
	size     int
	written  bool
	overflow bool
	limit    int64
}

func newCreationResponseCapture(writer gin.ResponseWriter, limit int64) *creationResponseCapture {
	return &creationResponseCapture{
		ResponseWriter: writer,
		status:         http.StatusOK,
		limit:          limit,
	}
}

func (writer *creationResponseCapture) WriteHeader(code int) {
	if writer.written {
		return
	}
	writer.status = code
	writer.written = true
}

func (writer *creationResponseCapture) WriteHeaderNow() {
	if !writer.written {
		writer.WriteHeader(writer.status)
	}
}

func (writer *creationResponseCapture) Write(data []byte) (int, error) {
	writer.WriteHeaderNow()
	if writer.limit > 0 && int64(writer.body.Len()+len(data)) > writer.limit {
		writer.overflow = true
		return 0, errCreationResponseTooLarge
	}
	written, err := writer.body.Write(data)
	writer.size += written
	return written, err
}

func (writer *creationResponseCapture) WriteString(data string) (int, error) {
	return writer.Write([]byte(data))
}

func (writer *creationResponseCapture) Status() int {
	return writer.status
}

func (writer *creationResponseCapture) Size() int {
	return writer.size
}

func (writer *creationResponseCapture) Written() bool {
	return writer.written
}

func (writer *creationResponseCapture) Flush() {
	writer.WriteHeaderNow()
}

func CreationImage(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	request, ok := middleware.GetCreationImageRequest(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid creation image request"})
		return
	}
	user, err := prepareCreationPlayground(c, request.Model)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": err.Error()})
		return
	}
	if !creationModelAvailable(user, request.Model, model.GenerationKindImage, request.Protocol, request.Group) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "the selected image model is not available"})
		return
	}

	parameters, err := common.Marshal(request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	reservedBytes := service.GenerationAssetLimit(model.GenerationKindImage) * int64(request.Count)
	job := &model.GenerationJob{
		UserID:     c.GetInt("id"),
		Kind:       model.GenerationKindImage,
		Protocol:   request.Protocol,
		Model:      request.Model,
		Prompt:     request.Prompt,
		Parameters: string(parameters),
	}
	if err := model.ReserveGenerationJob(job, reservedBytes); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	if request.ReferenceAssetID != "" {
		if _, err := model.AttachGenerationInputAsset(job.UserID, request.ReferenceAssetID, job.ID); err != nil {
			finishCreationJobWithError(job, http.StatusBadRequest, err)
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	responseLimit := reservedBytes + reservedBytes/2 + 2*1024*1024
	capture := newCreationResponseCapture(c.Writer, responseLimit)
	originalWriter := c.Writer
	c.Writer = capture
	if request.Protocol == "gemini-image" {
		Relay(c, types.RelayFormatGemini)
	} else {
		Relay(c, types.RelayFormatOpenAIImage)
	}
	c.Writer = originalWriter

	if capture.overflow {
		finishCreationJobWithError(job, http.StatusBadGateway, errCreationResponseTooLarge)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": errCreationResponseTooLarge.Error()})
		return
	}
	if capture.status < http.StatusOK || capture.status >= http.StatusMultipleChoices {
		err := errors.New(creationErrorMessage(capture.body.Bytes(), "image generation failed"))
		finishCreationJobWithError(job, capture.status, err)
		c.JSON(capture.status, gin.H{"success": false, "message": err.Error()})
		return
	}

	var assets []model.GenerationAsset
	if request.Protocol == "gemini-image" {
		assets, err = archiveGeminiCreationImages(job, capture.body.Bytes())
	} else {
		assets, err = archiveOpenAICreationImages(job, capture.body.Bytes())
	}
	if err != nil {
		finishCreationJobWithError(job, http.StatusBadGateway, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if len(assets) == 0 {
		err = errors.New("upstream returned no image")
		finishCreationJobWithError(job, http.StatusBadGateway, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.FinishGenerationJob(job.ID, model.GenerationJobSucceeded, "", ""); err != nil {
		common.ApiError(c, err)
		return
	}
	respondWithCreationJob(c, job.PublicID)
}

func CreationVideo(c *gin.Context) {
	if !requireCreationEnabled(c) {
		return
	}
	request, ok := middleware.GetCreationVideoRequest(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid creation video request"})
		return
	}
	user, err := prepareCreationPlayground(c, request.Model)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": err.Error()})
		return
	}
	if !creationModelAvailable(user, request.Model, model.GenerationKindVideo, "openai-video", request.Group) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "the selected video model is not available"})
		return
	}
	parameters, err := common.Marshal(request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	job := &model.GenerationJob{
		UserID:     c.GetInt("id"),
		Kind:       model.GenerationKindVideo,
		Protocol:   "openai-video",
		Model:      request.Model,
		Prompt:     request.Prompt,
		Parameters: string(parameters),
	}
	if err := model.ReserveVideoGenerationJob(
		job,
		service.GenerationAssetLimit(model.GenerationKindVideo),
		int64(creation_setting.GetSetting().MaxPendingVideoJobs),
	); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	if request.ReferenceAssetID != "" {
		if _, err := model.AttachGenerationInputAsset(job.UserID, request.ReferenceAssetID, job.ID); err != nil {
			finishCreationJobWithError(job, http.StatusBadRequest, err)
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	capture := newCreationResponseCapture(c.Writer, 2*1024*1024)
	originalWriter := c.Writer
	c.Writer = capture
	RelayTask(c)
	c.Writer = originalWriter
	if capture.overflow {
		finishCreationJobWithError(job, http.StatusBadGateway, errCreationResponseTooLarge)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": errCreationResponseTooLarge.Error()})
		return
	}
	if capture.status < http.StatusOK || capture.status >= http.StatusMultipleChoices {
		err := errors.New(creationErrorMessage(capture.body.Bytes(), "video generation failed"))
		finishCreationJobWithError(job, capture.status, err)
		c.JSON(capture.status, gin.H{"success": false, "message": err.Error()})
		return
	}
	taskID, err := creationVideoTaskID(capture.body.Bytes())
	if err != nil {
		finishCreationJobWithError(job, http.StatusBadGateway, err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if _, exists, err := model.GetByTaskId(c.GetInt("id"), taskID); err != nil || !exists {
		if err == nil {
			err = errors.New("video task was not persisted")
		}
		finishCreationJobWithError(job, http.StatusInternalServerError, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateGenerationJob(job.ID, map[string]any{
		"task_id":         taskID,
		"status":          model.GenerationJobQueued,
		"next_archive_at": time.Now().Add(15 * time.Second).Unix(),
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	respondWithCreationJob(c, job.PublicID)
}

func prepareCreationPlayground(c *gin.Context, modelName string) (*model.UserBase, error) {
	if c.GetBool("use_access_token") {
		return nil, errors.New("access-token sessions cannot use the creation center")
	}
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		return nil, err
	}
	user.WriteContext(c)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	token := &model.Token{
		UserId: user.Id,
		Name:   fmt.Sprintf("creation-%s", group),
		Group:  group,
	}
	if err := middleware.SetupContextForToken(c, token); err != nil {
		return nil, err
	}
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	return user, nil
}

func creationModelAvailable(user *model.UserBase, modelName string, kind string, protocol string, group string) bool {
	models, err := creationModelsForUser(user)
	if err != nil {
		return false
	}
	for _, candidate := range models {
		if candidate.ID != modelName || candidate.Kind != kind || candidate.Protocol != protocol {
			continue
		}
		return common.StringsContains(candidate.Groups, group)
	}
	return false
}

func archiveOpenAICreationImages(job *model.GenerationJob, body []byte) ([]model.GenerationAsset, error) {
	var response dto.ImageResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	assets := make([]model.GenerationAsset, 0, len(response.Data))
	for _, image := range response.Data {
		asset, err := archiveCreationImage(job, image.B64Json, image.Url)
		if err != nil {
			return assets, err
		}
		assets = append(assets, *asset)
	}
	return assets, nil
}

func archiveGeminiCreationImages(job *model.GenerationJob, body []byte) ([]model.GenerationAsset, error) {
	var response dto.GeminiChatResponse
	if err := common.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Gemini image response: %w", err)
	}
	assets := make([]model.GenerationAsset, 0)
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			asset, err := saveBase64CreationImage(job, part.InlineData.Data)
			if err != nil {
				return assets, err
			}
			assets = append(assets, *asset)
		}
	}
	return assets, nil
}

func archiveCreationImage(job *model.GenerationJob, encoded string, rawURL string) (*model.GenerationAsset, error) {
	if encoded != "" {
		return saveBase64CreationImage(job, encoded)
	}
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("upstream image result is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return service.SaveRemoteGenerationAsset(ctx, rawURL, service.GenerationAssetSaveRequest{
		UserID:             job.UserID,
		JobID:              job.ID,
		Role:               "output",
		Kind:               model.GenerationKindImage,
		MaxBytes:           service.GenerationAssetLimit(model.GenerationKindImage),
		ConsumeReservation: true,
	})
}

func saveBase64CreationImage(job *model.GenerationJob, encoded string) (*model.GenerationAsset, error) {
	if comma := strings.Index(encoded, ","); strings.HasPrefix(encoded, "data:") && comma >= 0 {
		encoded = encoded[comma+1:]
	}
	maxBytes := service.GenerationAssetLimit(model.GenerationKindImage)
	if int64(len(encoded)) > maxBytes*4/3+8 {
		return nil, errCreationResponseTooLarge
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode generated image: %w", err)
		}
	}
	return service.SaveGenerationAsset(service.GenerationAssetSaveRequest{
		UserID:             job.UserID,
		JobID:              job.ID,
		Role:               "output",
		Kind:               model.GenerationKindImage,
		Reader:             bytes.NewReader(decoded),
		MaxBytes:           maxBytes,
		ConsumeReservation: true,
	})
}

func creationVideoTaskID(body []byte) (string, error) {
	var response dto.OpenAIVideo
	if err := common.Unmarshal(body, &response); err == nil {
		if response.ID != "" {
			return response.ID, nil
		}
		if response.TaskID != "" {
			return response.TaskID, nil
		}
	}
	var generic map[string]any
	if err := common.Unmarshal(body, &generic); err != nil {
		return "", fmt.Errorf("decode video response: %w", err)
	}
	for _, key := range []string{"id", "task_id"} {
		if value, ok := generic[key].(string); ok && value != "" {
			return value, nil
		}
	}
	return "", errors.New("upstream video response did not contain a task id")
}

func finishCreationJobWithError(job *model.GenerationJob, status int, err error) {
	if job == nil {
		return
	}
	message := "creation failed"
	if err != nil {
		message = err.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	_ = model.FinishGenerationJob(job.ID, model.GenerationJobFailed, creationErrorCode(status), message)
}

func respondWithCreationJob(c *gin.Context, publicID string) {
	job, err := model.GetGenerationJobForUser(c.GetInt("id"), publicID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	decorateCreationJob(job)
	common.ApiSuccess(c, job)
}

var _ io.Writer = (*creationResponseCapture)(nil)
