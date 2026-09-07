package relay

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const creationImageExecutionStateKey = "creation_image_execution_state"

var creationImageBatchSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(authorization\s*[:=]\s*(?:bearer|basic)?\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)\b(bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password|cookie|set-cookie)["']?\s*[:=]\s*["']?)[^"'\s,;]+`),
}

type creationImageExecutionState struct {
	ReferenceCount int
	RequestedCount int
	Result         *CreationImageExecutionResult
}

type CreationImageExecutionResult struct {
	RelayInfo  *relaycommon.RelayInfo
	Usage      *dto.Usage
	LogContent []string
	BatchInfo  *relaycommon.ImageBatchInfo
	mu         sync.Mutex
	finalized  bool
}

func BeginCreationImageExecution(c *gin.Context, referenceCount int, requestedCount int) {
	if c == nil {
		return
	}
	c.Set(creationImageExecutionStateKey, &creationImageExecutionState{
		ReferenceCount: referenceCount,
		RequestedCount: requestedCount,
	})
}

func ApplyCreationImageCountToPrice(c *gin.Context, info *relaycommon.RelayInfo) error {
	state := getCreationImageExecutionState(c)
	if state == nil || info == nil || !info.PriceData.UsePrice || state.RequestedCount <= 0 {
		return nil
	}
	info.PriceData.AddOtherRatio("n", float64(state.RequestedCount))
	quota, err := common.QuotaFromFloatStrict(
		info.PriceData.ApplyOtherRatiosToFloat(
			info.PriceData.ModelPrice * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio,
		),
	)
	if err != nil {
		return err
	}
	info.PriceData.QuotaToPreConsume = quota
	return nil
}

func GetCreationImageExecutionResult(c *gin.Context) (*CreationImageExecutionResult, bool) {
	state := getCreationImageExecutionState(c)
	if state == nil || state.Result == nil {
		return nil, false
	}
	return state.Result, true
}

func AppendCreationImageExecutionError(result *CreationImageExecutionResult, index int, apiErr *types.NewAPIError) {
	if result == nil {
		return
	}
	appendCreationImageBatchError(result.BatchInfo, index, apiErr)
}

func CompleteCreationImageBilling(c *gin.Context, result *CreationImageExecutionResult, resultCount int) {
	if result == nil || result.RelayInfo == nil || result.BatchInfo == nil {
		return
	}
	if !finalizeCreationImageExecution(result) {
		return
	}
	result.BatchInfo.ResultCount = resultCount
	result.BatchInfo.FailedCount = result.BatchInfo.RequestedCount - resultCount
	if result.BatchInfo.FailedCount < 0 {
		result.BatchInfo.FailedCount = 0
	}
	if result.RelayInfo.PriceData.UsePrice && resultCount > 0 {
		result.RelayInfo.PriceData.AddOtherRatio("n", float64(resultCount))
	}
	service.PostTextConsumeQuota(c, result.RelayInfo, result.Usage, result.LogContent)
}

func RefundCreationImageBilling(c *gin.Context, result *CreationImageExecutionResult) {
	if result == nil || result.RelayInfo == nil || result.RelayInfo.Billing == nil {
		return
	}
	if !finalizeCreationImageExecution(result) {
		return
	}
	result.RelayInfo.Billing.Refund(c)
}

func finalizeCreationImageExecution(result *CreationImageExecutionResult) bool {
	result.mu.Lock()
	defer result.mu.Unlock()
	if result.finalized {
		return false
	}
	result.finalized = true
	return true
}

func getCreationImageExecutionState(c *gin.Context) *creationImageExecutionState {
	if c == nil {
		return nil
	}
	value, exists := c.Get(creationImageExecutionStateKey)
	if !exists {
		return nil
	}
	state, _ := value.(*creationImageExecutionState)
	return state
}

func storeCreationImageExecutionResult(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, logContent []string, batch *relaycommon.ImageBatchInfo) {
	state := getCreationImageExecutionState(c)
	if state == nil {
		return
	}
	if batch.ReferenceCount == 0 {
		batch.ReferenceCount = state.ReferenceCount
	}
	info.ImageBatchInfo = batch
	state.Result = &CreationImageExecutionResult{
		RelayInfo:  info,
		Usage:      usage,
		LogContent: append([]string(nil), logContent...),
		BatchInfo:  batch,
	}
}

func newCreationImageBatchInfo(c *gin.Context, info *relaycommon.RelayInfo, requestedCount int) *relaycommon.ImageBatchInfo {
	mode := dto.ImageBatchModeNative
	if info != nil && info.ChannelMeta != nil {
		mode = info.ChannelSetting.ImageBatchModeFor(info.OriginModelName, info.UpstreamModelName)
	}
	batch := &relaycommon.ImageBatchInfo{
		Mode:           mode,
		RequestedCount: requestedCount,
	}
	if state := getCreationImageExecutionState(c); state != nil {
		batch.ReferenceCount = state.ReferenceCount
	}
	return batch
}

func appendCreationImageBatchError(batch *relaycommon.ImageBatchInfo, index int, apiErr *types.NewAPIError) {
	if batch == nil || apiErr == nil {
		return
	}
	message := apiErr.MaskSensitiveError()
	for _, pattern := range creationImageBatchSecretPatterns {
		message = pattern.ReplaceAllString(message, "${1}***")
	}
	message = strings.Join(strings.Fields(message), " ")
	messageRunes := []rune(message)
	if len(messageRunes) > 240 {
		message = string(messageRunes[:240])
	}
	batch.Errors = append(batch.Errors, relaycommon.ImageBatchError{
		Index:      index,
		StatusCode: apiErr.StatusCode,
		Code:       string(apiErr.GetErrorCode()),
		Message:    message,
	})
}

func shouldStopCreationImageFanout(apiErr *types.NewAPIError) bool {
	if apiErr == nil {
		return false
	}
	if errorsIsContextCancellation(apiErr) {
		return true
	}
	switch apiErr.GetErrorCode() {
	case types.ErrorCodeChannelModelMappedError,
		types.ErrorCodeInvalidApiType,
		types.ErrorCodeInvalidRequest,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeChannelNoAvailableKey,
		types.ErrorCodeChannelInvalidKey,
		types.ErrorCodeChannelParamOverrideInvalid,
		types.ErrorCodeChannelHeaderOverrideInvalid:
		return true
	}
	if apiErr.StatusCode == http.StatusRequestTimeout {
		return false
	}
	return apiErr.StatusCode == http.StatusUnauthorized ||
		apiErr.StatusCode == http.StatusForbidden ||
		apiErr.StatusCode == http.StatusTooManyRequests ||
		(apiErr.StatusCode >= http.StatusBadRequest && apiErr.StatusCode < http.StatusInternalServerError)
}

func errorsIsContextCancellation(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

type creationImageResponseWriter struct {
	gin.ResponseWriter
	body    bytes.Buffer
	status  int
	written bool
}

func newCreationImageResponseWriter(parent gin.ResponseWriter) *creationImageResponseWriter {
	return &creationImageResponseWriter{ResponseWriter: parent, status: http.StatusOK}
}

func (writer *creationImageResponseWriter) WriteHeader(code int) {
	if writer.written {
		return
	}
	writer.status = code
	writer.written = true
}

func (writer *creationImageResponseWriter) WriteHeaderNow() {
	if !writer.written {
		writer.WriteHeader(writer.status)
	}
}

func (writer *creationImageResponseWriter) Write(data []byte) (int, error) {
	writer.WriteHeaderNow()
	return writer.body.Write(data)
}

func (writer *creationImageResponseWriter) WriteString(data string) (int, error) {
	return writer.Write([]byte(data))
}

func (writer *creationImageResponseWriter) Status() int {
	return writer.status
}

func (writer *creationImageResponseWriter) Size() int {
	return writer.body.Len()
}

func (writer *creationImageResponseWriter) Written() bool {
	return writer.written
}

func (writer *creationImageResponseWriter) Flush() {
	writer.WriteHeaderNow()
}

func saturatingAddUsageInt(left int, right int) int {
	if left < 0 {
		left = 0
	}
	if right <= 0 {
		return left
	}
	if left > math.MaxInt-right {
		return math.MaxInt
	}
	return left + right
}

func mergeCreationImageUsage(target *dto.Usage, addition *dto.Usage) {
	if target == nil || addition == nil {
		return
	}
	target.PromptTokens = saturatingAddUsageInt(target.PromptTokens, addition.PromptTokens)
	target.CompletionTokens = saturatingAddUsageInt(target.CompletionTokens, addition.CompletionTokens)
	target.TotalTokens = saturatingAddUsageInt(target.TotalTokens, addition.TotalTokens)
	target.PromptCacheHitTokens = saturatingAddUsageInt(target.PromptCacheHitTokens, addition.PromptCacheHitTokens)
	target.InputTokens = saturatingAddUsageInt(target.InputTokens, addition.InputTokens)
	target.OutputTokens = saturatingAddUsageInt(target.OutputTokens, addition.OutputTokens)
	target.ClaudeCacheCreation5mTokens = saturatingAddUsageInt(target.ClaudeCacheCreation5mTokens, addition.ClaudeCacheCreation5mTokens)
	target.ClaudeCacheCreation1hTokens = saturatingAddUsageInt(target.ClaudeCacheCreation1hTokens, addition.ClaudeCacheCreation1hTokens)
	mergeInputTokenDetails(&target.PromptTokensDetails, &addition.PromptTokensDetails)
	mergeOutputTokenDetails(&target.CompletionTokenDetails, &addition.CompletionTokenDetails)
	if addition.InputTokensDetails != nil {
		if target.InputTokensDetails == nil {
			target.InputTokensDetails = &dto.InputTokenDetails{}
		}
		mergeInputTokenDetails(target.InputTokensDetails, addition.InputTokensDetails)
	}
	if target.UsageSemantic == "" {
		target.UsageSemantic = addition.UsageSemantic
	}
	if target.UsageSource == "" {
		target.UsageSource = addition.UsageSource
	}
	mergeCreationBillingUsage(&target.BillingUsage, addition.BillingUsage)
}

func mergeInputTokenDetails(target *dto.InputTokenDetails, addition *dto.InputTokenDetails) {
	if target == nil || addition == nil {
		return
	}
	target.CachedTokens = saturatingAddUsageInt(target.CachedTokens, addition.CachedTokens)
	target.CachedCreationTokens = saturatingAddUsageInt(target.CachedCreationTokens, addition.CachedCreationTokens)
	target.CacheWriteTokens = saturatingAddUsageInt(target.CacheWriteTokens, addition.CacheWriteTokens)
	target.TextTokens = saturatingAddUsageInt(target.TextTokens, addition.TextTokens)
	target.AudioTokens = saturatingAddUsageInt(target.AudioTokens, addition.AudioTokens)
	target.ImageTokens = saturatingAddUsageInt(target.ImageTokens, addition.ImageTokens)
}

func mergeOutputTokenDetails(target *dto.OutputTokenDetails, addition *dto.OutputTokenDetails) {
	if target == nil || addition == nil {
		return
	}
	target.TextTokens = saturatingAddUsageInt(target.TextTokens, addition.TextTokens)
	target.AudioTokens = saturatingAddUsageInt(target.AudioTokens, addition.AudioTokens)
	target.ImageTokens = saturatingAddUsageInt(target.ImageTokens, addition.ImageTokens)
	target.ReasoningTokens = saturatingAddUsageInt(target.ReasoningTokens, addition.ReasoningTokens)
}

func mergeCreationBillingUsage(target **dto.BillingUsage, addition *dto.BillingUsage) {
	if addition == nil {
		return
	}
	if *target == nil {
		*target = &dto.BillingUsage{
			Source:    addition.Source,
			Semantic:  addition.Semantic,
			Estimated: addition.Estimated,
		}
	}
	if (*target).Source != addition.Source || (*target).Semantic != addition.Semantic {
		return
	}
	if addition.OpenAIUsage != nil {
		if (*target).OpenAIUsage == nil {
			(*target).OpenAIUsage = &dto.Usage{}
		}
		mergeCreationImageUsage((*target).OpenAIUsage, addition.OpenAIUsage)
	}
	if addition.ClaudeUsage != nil {
		mergeCreationClaudeUsage(&(*target).ClaudeUsage, addition.ClaudeUsage)
	}
	if addition.GeminiUsageMetadata != nil {
		mergeCreationGeminiUsage(&(*target).GeminiUsageMetadata, addition.GeminiUsageMetadata)
	}
	(*target).Estimated = (*target).Estimated || addition.Estimated
}

func mergeCreationClaudeUsage(target **dto.ClaudeUsage, addition *dto.ClaudeUsage) {
	if *target == nil {
		*target = &dto.ClaudeUsage{}
	}
	(*target).InputTokens = saturatingAddUsageInt((*target).InputTokens, addition.InputTokens)
	(*target).CacheCreationInputTokens = saturatingAddUsageInt((*target).CacheCreationInputTokens, addition.CacheCreationInputTokens)
	(*target).CacheReadInputTokens = saturatingAddUsageInt((*target).CacheReadInputTokens, addition.CacheReadInputTokens)
	(*target).OutputTokens = saturatingAddUsageInt((*target).OutputTokens, addition.OutputTokens)
	(*target).ClaudeCacheCreation5mTokens = saturatingAddUsageInt((*target).ClaudeCacheCreation5mTokens, addition.ClaudeCacheCreation5mTokens)
	(*target).ClaudeCacheCreation1hTokens = saturatingAddUsageInt((*target).ClaudeCacheCreation1hTokens, addition.ClaudeCacheCreation1hTokens)
	if addition.CacheCreation != nil {
		if (*target).CacheCreation == nil {
			(*target).CacheCreation = &dto.ClaudeCacheCreationUsage{}
		}
		(*target).CacheCreation.Ephemeral5mInputTokens = saturatingAddUsageInt((*target).CacheCreation.Ephemeral5mInputTokens, addition.CacheCreation.Ephemeral5mInputTokens)
		(*target).CacheCreation.Ephemeral1hInputTokens = saturatingAddUsageInt((*target).CacheCreation.Ephemeral1hInputTokens, addition.CacheCreation.Ephemeral1hInputTokens)
	}
	if addition.ServerToolUse != nil {
		if (*target).ServerToolUse == nil {
			(*target).ServerToolUse = &dto.ClaudeServerToolUse{}
		}
		(*target).ServerToolUse.WebSearchRequests = saturatingAddUsageInt((*target).ServerToolUse.WebSearchRequests, addition.ServerToolUse.WebSearchRequests)
	}
}

func mergeCreationGeminiUsage(target **dto.GeminiUsageMetadata, addition *dto.GeminiUsageMetadata) {
	if *target == nil {
		*target = &dto.GeminiUsageMetadata{}
	}
	(*target).PromptTokenCount = saturatingAddUsageInt((*target).PromptTokenCount, addition.PromptTokenCount)
	(*target).ToolUsePromptTokenCount = saturatingAddUsageInt((*target).ToolUsePromptTokenCount, addition.ToolUsePromptTokenCount)
	(*target).CandidatesTokenCount = saturatingAddUsageInt((*target).CandidatesTokenCount, addition.CandidatesTokenCount)
	(*target).TotalTokenCount = saturatingAddUsageInt((*target).TotalTokenCount, addition.TotalTokenCount)
	(*target).ThoughtsTokenCount = saturatingAddUsageInt((*target).ThoughtsTokenCount, addition.ThoughtsTokenCount)
	(*target).CachedContentTokenCount = saturatingAddUsageInt((*target).CachedContentTokenCount, addition.CachedContentTokenCount)
	(*target).PromptTokensDetails = appendNonNegativeGeminiTokenDetails((*target).PromptTokensDetails, addition.PromptTokensDetails)
	(*target).ToolUsePromptTokensDetails = appendNonNegativeGeminiTokenDetails((*target).ToolUsePromptTokensDetails, addition.ToolUsePromptTokensDetails)
	(*target).CandidatesTokensDetails = appendNonNegativeGeminiTokenDetails((*target).CandidatesTokensDetails, addition.CandidatesTokensDetails)
}

func appendNonNegativeGeminiTokenDetails(target []dto.GeminiPromptTokensDetails, addition []dto.GeminiPromptTokensDetails) []dto.GeminiPromptTokensDetails {
	for _, detail := range addition {
		if detail.TokenCount <= 0 {
			continue
		}
		target = append(target, detail)
	}
	return target
}

func creationImageEmptyResponseError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New("upstream returned no image"),
		types.ErrorCodeEmptyResponse,
		http.StatusBadGateway,
	)
}

func writeCreationImageResponse(c *gin.Context, body []byte) *types.NewAPIError {
	if c == nil {
		return types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	c.Status(http.StatusOK)
	if _, err := c.Writer.Write(body); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	return nil
}
