package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

// outboundImageCounter is an image adaptor whose upstream body carries the
// image count in a member other than n. OutboundImageCount reads the count
// from that body, nil when the member is absent.
type outboundImageCounter interface {
	OutboundImageCount(body []byte) (*uint, error)
}

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	info.BillingImageCount = nil
	info.ImageRequestCount = 0

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	applyCreationImageReferenceSemantics(request, info, getCreationImageExecutionState(c))

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	imageCount, err := request.ImageCount(false)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	batch := newCreationImageBatchInfo(c, info, imageCount)
	if getCreationImageExecutionState(c) != nil && batch.Mode == dto.ImageBatchModeFanout && imageCount > 1 {
		return runOpenAIImageFanout(c, info, request, adaptor, batch)
	}

	usage, newAPIError := executeOpenAIImageRequest(c, info, request, adaptor, false)
	if newAPIError != nil {
		if getCreationImageExecutionState(c) != nil {
			appendCreationImageBatchError(batch, 1, newAPIError)
			batch.FailedCount = batch.RequestedCount
			storeCreationImageExecutionResult(c, info, nil, nil, batch)
		}
		return newAPIError
	}
	normalizeCreationImageUsage(usage)

	quality := request.Quality
	if quality == "" {
		quality = "standard"
	}

	logContent := make([]*common.Message, 0, 3)
	if len(request.Size) > 0 {
		logContent = append(logContent, common.NewMessage("Size {{size}}", map[string]any{"size": request.Size}))
	}
	if len(quality) > 0 {
		logContent = append(logContent, common.NewMessage("Quality {{quality}}", map[string]any{"quality": quality}))
	}
	imageN := info.RequestedImageCount()
	if info.BillingImageCount != nil {
		imageN = *info.BillingImageCount
	}
	if imageN > 0 {
		logContent = append(logContent, common.NewMessage("Image count {{count}}", map[string]any{"count": imageN}))
	}

	if getCreationImageExecutionState(c) != nil {
		storeCreationImageExecutionResult(c, info, usage, logContent, batch)
		return nil
	}
	service.PostTextConsumeQuota(c, info, usage, logContent)
	return nil
}

func executeOpenAIImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, adaptor channel.Adaptor, forceConvert bool) (*dto.Usage, *types.NewAPIError) {
	imageCount, err := request.ImageCount(false)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	var apiErr *types.NewAPIError
	var requestBody io.Reader
	var jsonData []byte
	var outbound struct {
		N *uint `json:"n"`
	}
	validateQuantity := false

	if !forceConvert && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if apiErr = requestplugin.PassThrough(c, info, storage); apiErr != nil {
			return nil, apiErr
		}
		if !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			counter, countsOwnMember := adaptor.(outboundImageCounter)
			switch {
			case countsOwnMember:
				// The adaptor sends the count in its own member, which only
				// it can read, so it reads the whole body.
				var raw []byte
				raw, err = storage.Bytes()
				if err == nil {
					err = common.Unmarshal(raw, &outbound)
				}
				if err == nil {
					outbound.N, err = counter.OutboundImageCount(raw)
				}
			case storage.IsDisk():
				reader, readErr := storage.NewReader()
				if readErr != nil {
					return nil, types.NewErrorWithStatusCode(readErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				}
				err = common.DecodeJsonBody(reader, &outbound)
				_ = reader.Close()
			default:
				var raw []byte
				raw, err = storage.Bytes()
				if err == nil {
					err = common.Unmarshal(raw, &outbound)
				}
			}
			if err != nil {
				return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid image billing parameters: %w", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			validateQuantity = true
			if common.DebugEnabled {
				if raw, readErr := storage.Bytes(); readErr == nil {
					logger.LogDebug(c, "image request body: %s", raw)
				}
			}
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			// An adaptor that already classified its rejection (status code
			// and retry policy) keeps that classification instead of being
			// downgraded to a retryable conversion failure.
			var apiErr *types.NewAPIError
			if errors.As(err, &apiErr) {
				return nil, apiErr
			}
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			if apiErr = requestplugin.PassThrough(c, info, nil); apiErr != nil {
				return nil, apiErr
			}
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err = common.Marshal(convertedRequest)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			if jsonData, apiErr = requestplugin.Outbound(c, info, jsonData); apiErr != nil {
				return nil, apiErr
			}
			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return nil, newAPIErrorFromParamOverride(err)
				}
			}
			if forceConvert {
				jsonData, err = sjson.SetBytes(jsonData, "n", 1)
				if err != nil {
					return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
				}
			}
			if apiErr = requestplugin.Egress(c, info, jsonData); apiErr != nil {
				return nil, apiErr
			}

		}
	}
	if jsonData != nil {
		// This is a different trust boundary from ingress: channel overrides
		// and request plugins can change the quantity actually submitted, in
		// n or in the member the adaptor sends it in.
		err := common.Unmarshal(jsonData, &outbound)
		if counter, ok := adaptor.(outboundImageCounter); ok && err == nil {
			outbound.N, err = counter.OutboundImageCount(jsonData)
		}
		if err != nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid image billing parameters: %w", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		validateQuantity = true
	}
	if validateQuantity {
		// Validate the count in the body sent upstream, including channel overrides.
		quantityRequest := dto.ImageRequest{N: outbound.N}
		if quantityRequest.N == nil {
			quantityRequest.N = common.GetPointer(uint(imageCount))
		}
		imageCount, err = quantityRequest.ImageCount(false)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
	}
	if forceConvert && imageCount != 1 {
		return nil, types.NewErrorWithStatusCode(errors.New("image fanout requires one output per attempt"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if jsonData != nil {
		logger.LogDebug(c, "image request body: %s", jsonData)
		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		requestBody = body
	}

	if !forceConvert {
		if billingErr := service.PrepareImageBillingForRequest(c, info, imageCount); billingErr != nil {
			return nil, billingErr
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return nil, newAPIError
			}
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return nil, newAPIError
	}
	return usage.(*dto.Usage), nil
}

func runOpenAIImageFanout(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, adaptor channel.Adaptor, batch *relaycommon.ImageBatchInfo) *types.NewAPIError {
	requestedCount := batch.RequestedCount
	if billingErr := service.PrepareImageBillingForRequest(c, info, requestedCount); billingErr != nil {
		return billingErr
	}
	request.N = common.GetPointer(uint(1))
	aggregatedResponse := dto.ImageResponse{}
	aggregatedUsage := &dto.Usage{}
	var lastError *types.NewAPIError

	for index := 1; index <= requestedCount && len(aggregatedResponse.Data) < requestedCount; index++ {
		if err := c.Request.Context().Err(); err != nil {
			lastError = types.NewErrorWithStatusCode(err, types.ErrorCodeDoRequestFailed, http.StatusRequestTimeout, types.ErrOptionWithSkipRetry())
			appendCreationImageBatchError(batch, index, lastError)
			break
		}
		attemptContext := c.Copy()
		attemptContext.Request = c.Request.Clone(c.Request.Context())
		attemptWriter := newCreationImageResponseWriter(c.Writer)
		attemptContext.Writer = attemptWriter
		attemptRequest, err := common.DeepCopy(request)
		if err != nil {
			lastError = types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
			appendCreationImageBatchError(batch, index, lastError)
			break
		}
		usage, apiErr := executeOpenAIImageRequest(attemptContext, info, attemptRequest, adaptor, true)
		if apiErr != nil {
			lastError = apiErr
			appendCreationImageBatchError(batch, index, apiErr)
			if shouldStopCreationImageFanout(apiErr) {
				break
			}
			continue
		}

		var response dto.ImageResponse
		if err := common.Unmarshal(attemptWriter.body.Bytes(), &response); err != nil {
			lastError = types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			appendCreationImageBatchError(batch, index, lastError)
			continue
		}
		response.Data = slices.DeleteFunc(response.Data, func(image dto.ImageData) bool {
			return image.Url == "" && image.B64Json == ""
		})
		if len(response.Data) == 0 {
			lastError = creationImageEmptyResponseError()
			appendCreationImageBatchError(batch, index, lastError)
			continue
		}
		remaining := requestedCount - len(aggregatedResponse.Data)
		if len(response.Data) > remaining {
			response.Data = response.Data[:remaining]
		}
		if aggregatedResponse.Created == 0 {
			aggregatedResponse.Created = response.Created
			aggregatedResponse.Metadata = response.Metadata
		}
		aggregatedResponse.Data = append(aggregatedResponse.Data, response.Data...)
		mergeCreationImageUsage(aggregatedUsage, usage)
	}

	batch.ResultCount = len(aggregatedResponse.Data)
	batch.FailedCount = requestedCount - batch.ResultCount
	logContent := []*common.Message{common.NewMessage("Image count {{count}}", map[string]any{"count": batch.ResultCount})}
	if batch.ResultCount == 0 {
		storeCreationImageExecutionResult(c, info, nil, logContent, batch)
		if lastError == nil {
			lastError = creationImageEmptyResponseError()
		}
		return types.NewErrorWithStatusCode(lastError, lastError.GetErrorCode(), http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	normalizeCreationImageUsage(aggregatedUsage)
	storeCreationImageExecutionResult(c, info, aggregatedUsage, logContent, batch)

	body, err := common.Marshal(aggregatedResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	c.Header("Content-Type", "application/json")
	return writeCreationImageResponse(c, body)
}

func normalizeCreationImageUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = 1
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = 1
	}
}

func supportsHighInputFidelity(modelName string) bool {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "gpt-image-1", "gpt-image-1.5", "gpt-image-2":
		return true
	default:
		return false
	}
}

func applyCreationImageReferenceSemantics(request *dto.ImageRequest, info *relaycommon.RelayInfo, state *creationImageExecutionState) {
	if request == nil || info == nil || state == nil || state.ReferenceCount <= 0 {
		return
	}
	request.Prompt += "\n\nUse all reference images in their upload order when producing the result."
	if supportsHighInputFidelity(info.UpstreamModelName) {
		request.InputFidelity = []byte(`"high"`)
	}
}
