package relay

import (
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
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

func isNoThinkingRequest(req *dto.GeminiChatRequest) bool {
	if req.GenerationConfig.ThinkingConfig != nil && req.GenerationConfig.ThinkingConfig.ThinkingBudget != nil {
		configBudget := req.GenerationConfig.ThinkingConfig.ThinkingBudget
		if configBudget != nil && *configBudget == 0 {
			// 如果思考预算为 0，则认为是非思考请求
			return true
		}
	}
	return false
}

func trimModelThinking(modelName string) string {
	// 去除模型名称中的 -nothinking 后缀
	if strings.HasSuffix(modelName, "-nothinking") {
		return strings.TrimSuffix(modelName, "-nothinking")
	}
	// 去除模型名称中的 -thinking 后缀
	if strings.HasSuffix(modelName, "-thinking") {
		return strings.TrimSuffix(modelName, "-thinking")
	}

	// 去除模型名称中的 -thinking-number
	if strings.Contains(modelName, "-thinking-") {
		parts := strings.Split(modelName, "-thinking-")
		if len(parts) > 1 {
			return parts[0] + "-thinking"
		}
	}
	return modelName
}

func GeminiHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	geminiReq, ok := info.Request.(*dto.GeminiChatRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.GeminiChatRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(geminiReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeminiChatRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// model mapped 模型映射
	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled {
		if isNoThinkingRequest(request) {
			// check is thinking
			if !strings.Contains(info.OriginModelName, "-nothinking") {
				// try to get no thinking model price
				noThinkingModelName := info.OriginModelName + "-nothinking"
				containPrice := helper.HasModelBillingConfig(noThinkingModelName)
				if containPrice {
					info.OriginModelName = noThinkingModelName
					info.UpstreamModelName = noThinkingModelName
				}
			}
		}
		if request.GenerationConfig.ThinkingConfig == nil {
			relayconvert.ApplyGeminiThinkingConfig(request, info)
		}
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}

	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.SystemInstructions == nil {
			request.SystemInstructions = &dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{Text: info.ChannelSetting.SystemPrompt},
				},
			}
		} else if len(request.SystemInstructions.Parts) == 0 {
			request.SystemInstructions.Parts = []dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			merged := false
			for i := range request.SystemInstructions.Parts {
				if request.SystemInstructions.Parts[i].Text == "" {
					continue
				}
				request.SystemInstructions.Parts[i].Text = info.ChannelSetting.SystemPrompt + "\n" + request.SystemInstructions.Parts[i].Text
				merged = true
				break
			}
			if !merged {
				request.SystemInstructions.Parts = append([]dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}, request.SystemInstructions.Parts...)
			}
		}
	}

	// Clean up empty system instruction
	if request.SystemInstructions != nil {
		hasContent := false
		for _, part := range request.SystemInstructions.Parts {
			if part.Text != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			request.SystemInstructions = nil
		}
	}

	if state := getCreationImageExecutionState(c); state != nil && state.ReferenceCount > 0 {
		appendGeminiReferenceInstruction(request, state.ReferenceCount)
	}
	requestedCount := 1
	if request.GenerationConfig.CandidateCount != nil && *request.GenerationConfig.CandidateCount > 0 {
		requestedCount = *request.GenerationConfig.CandidateCount
	}
	batch := newCreationImageBatchInfo(c, info, requestedCount)
	if getCreationImageExecutionState(c) != nil && batch.Mode == dto.ImageBatchModeFanout && requestedCount > 1 {
		return runGeminiImageFanout(c, info, request, adaptor, batch)
	}

	usage, apiErr := executeGeminiRequest(c, info, request, adaptor, false)
	if apiErr != nil {
		if getCreationImageExecutionState(c) != nil {
			appendCreationImageBatchError(batch, 1, apiErr)
			batch.FailedCount = batch.RequestedCount
			storeCreationImageExecutionResult(c, info, nil, nil, batch)
		}
		return apiErr
	}
	if getCreationImageExecutionState(c) != nil {
		storeCreationImageExecutionResult(c, info, usage, nil, batch)
		return nil
	}
	service.PostTextConsumeQuota(c, info, usage, nil)
	return nil
}

func executeGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest, adaptor channel.Adaptor, forceConvert bool) (*dto.Usage, *types.NewAPIError) {

	var requestBody io.Reader
	if !forceConvert && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		// 使用 ConvertGeminiRequest 转换请求格式
		convertedRequest, err := adaptor.ConvertGeminiRequest(c, info, request)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
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
			jsonData, err = sjson.SetBytes(jsonData, "generationConfig.candidateCount", 1)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
		}

		logger.LogDebug(c, "Gemini request body: %s", jsonData)

		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return nil, newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		return nil, openaiErr
	}
	return usage.(*dto.Usage), nil
}

func appendGeminiReferenceInstruction(request *dto.GeminiChatRequest, referenceCount int) {
	if request == nil || referenceCount <= 0 {
		return
	}
	instruction := fmt.Sprintf("Use all %d reference images in their upload order when producing the result.", referenceCount)
	for contentIndex := range request.Contents {
		for partIndex := range request.Contents[contentIndex].Parts {
			part := &request.Contents[contentIndex].Parts[partIndex]
			if part.Text == "" {
				continue
			}
			part.Text += "\n\n" + instruction
			return
		}
	}
}

func runGeminiImageFanout(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest, adaptor channel.Adaptor, batch *relaycommon.ImageBatchInfo) *types.NewAPIError {
	requestedCount := batch.RequestedCount
	request.GenerationConfig.CandidateCount = common.GetPointer(1)
	aggregatedResponse := dto.GeminiChatResponse{}
	aggregatedUsage := &dto.Usage{}
	var aggregatedMetadata *dto.GeminiUsageMetadata
	var lastError *types.NewAPIError
	resultCount := 0

	for index := 1; index <= requestedCount && resultCount < requestedCount; index++ {
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
		usage, apiErr := executeGeminiRequest(attemptContext, info, attemptRequest, adaptor, true)
		if apiErr != nil {
			lastError = apiErr
			appendCreationImageBatchError(batch, index, apiErr)
			if shouldStopCreationImageFanout(apiErr) {
				break
			}
			continue
		}

		var response dto.GeminiChatResponse
		if err := common.Unmarshal(attemptWriter.body.Bytes(), &response); err != nil {
			lastError = types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			appendCreationImageBatchError(batch, index, lastError)
			continue
		}
		filteredCandidates, imageCount := creationGeminiImageCandidates(response.Candidates, requestedCount-resultCount)
		if imageCount == 0 {
			lastError = creationImageEmptyResponseError()
			appendCreationImageBatchError(batch, index, lastError)
			continue
		}
		if aggregatedResponse.PromptFeedback == nil {
			aggregatedResponse.PromptFeedback = response.PromptFeedback
		}
		aggregatedResponse.Candidates = append(aggregatedResponse.Candidates, filteredCandidates...)
		resultCount += imageCount
		mergeCreationImageUsage(aggregatedUsage, usage)
		if response.HasUsageMetadata {
			mergeCreationGeminiUsage(&aggregatedMetadata, &response.UsageMetadata)
		}
	}

	batch.ResultCount = resultCount
	batch.FailedCount = requestedCount - resultCount
	if resultCount == 0 {
		storeCreationImageExecutionResult(c, info, nil, nil, batch)
		if lastError == nil {
			lastError = creationImageEmptyResponseError()
		}
		return types.NewErrorWithStatusCode(lastError, lastError.GetErrorCode(), http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	storeCreationImageExecutionResult(c, info, aggregatedUsage, nil, batch)
	if aggregatedMetadata != nil {
		aggregatedResponse.UsageMetadata = *aggregatedMetadata
		aggregatedResponse.HasUsageMetadata = true
	}
	body, err := common.Marshal(aggregatedResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	c.Header("Content-Type", "application/json")
	return writeCreationImageResponse(c, body)
}

func creationGeminiImageCandidates(candidates []dto.GeminiChatCandidate, limit int) ([]dto.GeminiChatCandidate, int) {
	if limit <= 0 {
		return nil, 0
	}
	filtered := make([]dto.GeminiChatCandidate, 0, len(candidates))
	count := 0
	for _, candidate := range candidates {
		parts := make([]dto.GeminiPart, 0, len(candidate.Content.Parts))
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" || count >= limit {
				continue
			}
			parts = append(parts, part)
			count++
		}
		if len(parts) == 0 {
			continue
		}
		candidate.Content.Parts = parts
		candidate.Index = int64(len(filtered))
		filtered = append(filtered, candidate)
	}
	return filtered, count
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	isBatch := strings.HasSuffix(c.Request.URL.Path, "batchEmbedContents")
	info.IsGeminiBatchEmbedding = isBatch

	var req dto.Request
	var err error
	var inputTexts []string

	if isBatch {
		batchRequest := &dto.GeminiBatchEmbeddingRequest{}
		err = common.UnmarshalBodyReusable(c, batchRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = batchRequest
		for _, r := range batchRequest.Requests {
			for _, part := range r.Content.Parts {
				if part.Text != "" {
					inputTexts = append(inputTexts, part.Text)
				}
			}
		}
	} else {
		singleRequest := &dto.GeminiEmbeddingRequest{}
		err = common.UnmarshalBodyReusable(c, singleRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = singleRequest
		for _, part := range singleRequest.Content.Parts {
			if part.Text != "" {
				inputTexts = append(inputTexts, part.Text)
			}
		}
	}

	err = helper.ModelMappedHelper(c, info, req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	req.SetModelName("models/" + info.UpstreamModelName)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader
	jsonData, err := common.Marshal(req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	// apply param override
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return newAPIErrorFromParamOverride(err)
		}
	}
	logger.LogDebug(c, "Gemini embedding request body: %s", jsonData)
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	requestBody = body

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		return openaiErr
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}
