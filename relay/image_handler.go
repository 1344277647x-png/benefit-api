package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

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

	imageN := uint(1)
	if request.N != nil {
		imageN = *request.N
	}
	batch := newCreationImageBatchInfo(c, info, int(imageN))
	if getCreationImageExecutionState(c) != nil && batch.Mode == dto.ImageBatchModeFanout && imageN > 1 {
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

	logContent := make([]string, 0, 3)
	if len(request.Size) > 0 {
		logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
	}
	if len(quality) > 0 {
		logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
	}
	if imageN > 0 {
		logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
	}

	if getCreationImageExecutionState(c) != nil {
		storeCreationImageExecutionResult(c, info, usage, logContent, batch)
		return nil
	}
	service.PostTextConsumeQuota(c, info, usage, logContent)
	return nil
}

func executeOpenAIImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, adaptor channel.Adaptor, forceConvert bool) (*dto.Usage, *types.NewAPIError) {
	var requestBody io.Reader

	if !forceConvert && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
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

			logger.LogDebug(c, "image request body: %s", jsonData)
			body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			defer closer.Close()
			jsonData = nil
			requestBody = body
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
	logContent := []string{fmt.Sprintf("生成数量 %d", requestedCount)}
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
