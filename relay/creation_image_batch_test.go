package relay

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type creationImageAttempt struct {
	body  string
	usage *dto.Usage
	err   *types.NewAPIError
}

type creationImageTestAdaptor struct {
	channel.Adaptor
	attempts      []creationImageAttempt
	requestBodies [][]byte
}

type creationImageRecordingBilling struct {
	refundCalls int
}

func (*creationImageRecordingBilling) Settle(int) error { return nil }

func (b *creationImageRecordingBilling) Refund(*gin.Context) { b.refundCalls++ }

func (*creationImageRecordingBilling) NeedsRefund() bool { return true }

func (*creationImageRecordingBilling) GetPreConsumedQuota() int { return 0 }

func (*creationImageRecordingBilling) Reserve(int) error { return nil }

func (a *creationImageTestAdaptor) ConvertImageRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *creationImageTestAdaptor) ConvertGeminiRequest(_ *gin.Context, _ *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return request, nil
}

func (a *creationImageTestAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	index := len(a.requestBodies)
	a.requestBodies = append(a.requestBodies, body)
	attempt := a.attempts[index]
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Test-Attempt": []string{string(rune(index + 1))}},
		Body:       io.NopCloser(strings.NewReader(attempt.body)),
	}, nil
}

func (a *creationImageTestAdaptor) DoResponse(c *gin.Context, response *http.Response, _ *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	index := int([]rune(response.Header.Get("X-Test-Attempt"))[0]) - 1
	attempt := a.attempts[index]
	if attempt.err != nil {
		return nil, attempt.err
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if _, err := c.Writer.Write(body); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	return attempt.usage, nil
}

func newCreationImageTestContext(t *testing.T, requestedCount int, referenceCount int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/pg/creation/images", nil)
	BeginCreationImageExecution(c, referenceCount, requestedCount)
	return c, recorder
}

func newCreationImageTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: "public-image",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-image-2",
		},
	}
}

func TestMergeCreationImageUsageAggregatesCanonicalAndBillingUsage(t *testing.T) {
	first := &dto.Usage{
		PromptTokens:     math.MaxInt - 2,
		CompletionTokens: 3,
		InputTokensDetails: &dto.InputTokenDetails{
			ImageTokens: 4,
		},
		BillingUsage: dto.NewGeminiChatBillingUsage(&dto.GeminiUsageMetadata{
			PromptTokenCount: 5,
			PromptTokensDetails: []dto.GeminiPromptTokensDetails{
				{Modality: "IMAGE", TokenCount: 4},
			},
		}),
	}
	second := &dto.Usage{
		PromptTokens:     10,
		CompletionTokens: 7,
		OutputTokens:     -5,
		InputTokensDetails: &dto.InputTokenDetails{
			ImageTokens: 6,
		},
		BillingUsage: dto.NewGeminiChatBillingUsage(&dto.GeminiUsageMetadata{
			PromptTokenCount: 8,
			PromptTokensDetails: []dto.GeminiPromptTokensDetails{
				{Modality: "IMAGE", TokenCount: 6},
				{Modality: "TEXT", TokenCount: -50},
			},
		}),
	}

	merged := &dto.Usage{}
	mergeCreationImageUsage(merged, first)
	mergeCreationImageUsage(merged, second)

	assert.Equal(t, math.MaxInt, merged.PromptTokens)
	assert.Equal(t, 10, merged.CompletionTokens)
	assert.Zero(t, merged.OutputTokens)
	require.NotNil(t, merged.InputTokensDetails)
	assert.Equal(t, 10, merged.InputTokensDetails.ImageTokens)
	require.NotNil(t, merged.BillingUsage)
	require.NotNil(t, merged.BillingUsage.GeminiUsageMetadata)
	assert.Equal(t, 13, merged.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	assert.Len(t, merged.BillingUsage.GeminiUsageMetadata.PromptTokensDetails, 2)
	assert.Equal(t, 4, merged.BillingUsage.GeminiUsageMetadata.PromptTokensDetails[0].TokenCount)
	assert.Equal(t, 6, merged.BillingUsage.GeminiUsageMetadata.PromptTokensDetails[1].TokenCount)
}

func TestCreationImageFixedPriceUsesRequestedCountThenActualResultCount(t *testing.T) {
	c, _ := newCreationImageTestContext(t, 4, 0)
	info := &relaycommon.RelayInfo{PriceData: hosttypes.PriceData{
		UsePrice:       true,
		ModelPrice:     0.02,
		GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1.5},
	}}

	require.NoError(t, ApplyCreationImageCountToPrice(c, info))
	assert.Equal(t, 60_000, info.PriceData.QuotaToPreConsume)
	assert.Equal(t, 4.0, info.PriceData.OtherRatios()["n"])

	result := &CreationImageExecutionResult{
		RelayInfo: info,
		BatchInfo: &relaycommon.ImageBatchInfo{RequestedCount: 4},
	}
	result.BatchInfo.ResultCount = 2
	result.BatchInfo.FailedCount = 2
	result.RelayInfo.PriceData.AddOtherRatio("n", 2)
	assert.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
}

func TestRefundCreationImageBillingOnlyCallsBillingOnce(t *testing.T) {
	billing := &creationImageRecordingBilling{}
	result := &CreationImageExecutionResult{
		RelayInfo: &relaycommon.RelayInfo{Billing: billing},
		BatchInfo: &relaycommon.ImageBatchInfo{},
	}

	RefundCreationImageBilling(nil, result)
	RefundCreationImageBilling(nil, result)

	assert.Equal(t, 1, billing.refundCalls)
}

func TestShouldStopCreationImageFanoutClassifiesErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   types.ErrorCode
		stop   bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, stop: true},
		{name: "rate limited", status: http.StatusTooManyRequests, stop: true},
		{name: "request timeout", status: http.StatusRequestTimeout, stop: false},
		{name: "server error", status: http.StatusBadGateway, stop: false},
		{name: "mapping error", status: http.StatusInternalServerError, code: types.ErrorCodeChannelModelMappedError, stop: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := types.NewErrorWithStatusCode(assert.AnError, test.code, test.status)
			assert.Equal(t, test.stop, shouldStopCreationImageFanout(err))
		})
	}
}

func TestAppendCreationImageBatchErrorMasksAndTruncatesSummary(t *testing.T) {
	batch := &relaycommon.ImageBatchInfo{}
	apiErr := types.NewErrorWithStatusCode(
		errors.New("Authorization: Bearer demo-value api_key='demo-key' password=demo-pass "+strings.Repeat("中", 400)),
		types.ErrorCodeBadResponse,
		http.StatusBadGateway,
	)

	appendCreationImageBatchError(batch, 1, apiErr)

	require.Len(t, batch.Errors, 1)
	assert.NotContains(t, batch.Errors[0].Message, "demo-value")
	assert.NotContains(t, batch.Errors[0].Message, "demo-key")
	assert.NotContains(t, batch.Errors[0].Message, "demo-pass")
	assert.LessOrEqual(t, len([]rune(batch.Errors[0].Message)), 240)
	assert.True(t, strings.Contains(batch.Errors[0].Message, "***"))
	assert.True(t, strings.ToValidUTF8(batch.Errors[0].Message, "") == batch.Errors[0].Message)
}

func TestOpenAIImageFanoutContinuesRecoverableFailuresAndClipsExcessResults(t *testing.T) {
	c, recorder := newCreationImageTestContext(t, 4, 2)
	info := newCreationImageTestRelayInfo()
	adaptor := &creationImageTestAdaptor{attempts: []creationImageAttempt{
		{body: `{"data":[{"b64_json":"one"}]}`, usage: &dto.Usage{PromptTokens: 2, TotalTokens: 2}},
		{err: types.NewErrorWithStatusCode(assert.AnError, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)},
		{body: `not-json`, usage: &dto.Usage{PromptTokens: 100, TotalTokens: 100}},
		{body: `{"data":[{"b64_json":"two"},{"b64_json":"three"},{"b64_json":"four"},{"b64_json":"excess"}]}`, usage: &dto.Usage{PromptTokens: 3, TotalTokens: 3}},
	}}
	request := &dto.ImageRequest{Model: "public-image", Prompt: "use both references", N: common.GetPointer(uint(4))}
	batch := &relaycommon.ImageBatchInfo{Mode: dto.ImageBatchModeFanout, RequestedCount: 4, ReferenceCount: 2}

	apiErr := runOpenAIImageFanout(c, info, request, adaptor, batch)

	require.Nil(t, apiErr)
	assert.Equal(t, 4, batch.ResultCount)
	assert.Zero(t, batch.FailedCount)
	assert.Len(t, batch.Errors, 2)
	assert.Len(t, adaptor.requestBodies, 4)
	for _, body := range adaptor.requestBodies {
		assert.Contains(t, string(body), `"n":1`)
		assert.Contains(t, string(body), "use both references")
	}
	var response dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 4)
	assert.Equal(t, "one", response.Data[0].B64Json)
	assert.Equal(t, "two", response.Data[1].B64Json)
	assert.Equal(t, "four", response.Data[3].B64Json)
	result, ok := GetCreationImageExecutionResult(c)
	require.True(t, ok)
	assert.Equal(t, 5, result.Usage.PromptTokens)
}

func TestOpenAIImageFanoutStopsAfterRateLimitAndReturnsExistingResult(t *testing.T) {
	c, _ := newCreationImageTestContext(t, 4, 0)
	info := newCreationImageTestRelayInfo()
	adaptor := &creationImageTestAdaptor{attempts: []creationImageAttempt{
		{body: `{"data":[{"b64_json":"one"}]}`, usage: &dto.Usage{PromptTokens: 2, TotalTokens: 2}},
		{err: types.NewErrorWithStatusCode(assert.AnError, types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)},
	}}
	batch := &relaycommon.ImageBatchInfo{Mode: dto.ImageBatchModeFanout, RequestedCount: 4}

	apiErr := runOpenAIImageFanout(c, info, &dto.ImageRequest{N: common.GetPointer(uint(4))}, adaptor, batch)

	require.Nil(t, apiErr)
	assert.Len(t, adaptor.requestBodies, 2)
	assert.Equal(t, 1, batch.ResultCount)
	assert.Equal(t, 3, batch.FailedCount)
}

func TestOpenAIImageFanoutZeroResultsReturnsBadGateway(t *testing.T) {
	c, _ := newCreationImageTestContext(t, 2, 0)
	info := newCreationImageTestRelayInfo()
	adaptor := &creationImageTestAdaptor{attempts: []creationImageAttempt{
		{body: `{"data":[]}`, usage: &dto.Usage{PromptTokens: 9, TotalTokens: 9}},
		{body: `{"data":[]}`, usage: &dto.Usage{PromptTokens: 9, TotalTokens: 9}},
	}}
	batch := &relaycommon.ImageBatchInfo{Mode: dto.ImageBatchModeFanout, RequestedCount: 2}

	apiErr := runOpenAIImageFanout(c, info, &dto.ImageRequest{N: common.GetPointer(uint(2))}, adaptor, batch)

	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Zero(t, batch.ResultCount)
	assert.Equal(t, 2, batch.FailedCount)
	result, ok := GetCreationImageExecutionResult(c)
	require.True(t, ok)
	assert.Nil(t, result.Usage)
	assert.Len(t, batch.Errors, 2)
}

func TestGeminiImageFanoutPreservesOrderedReferencesAndAllImageParts(t *testing.T) {
	c, recorder := newCreationImageTestContext(t, 3, 2)
	info := newCreationImageTestRelayInfo()
	adaptor := &creationImageTestAdaptor{attempts: []creationImageAttempt{
		{body: `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"one"}}]}},{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"two"}}]}}]}`, usage: &dto.Usage{PromptTokens: 2, TotalTokens: 2}},
		{body: `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"three"}},{"inlineData":{"mimeType":"image/png","data":"excess"}}]}}]}`, usage: &dto.Usage{PromptTokens: 3, TotalTokens: 3}},
	}}
	request := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{
			{Text: "compose"},
			{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "reference-one"}},
			{InlineData: &dto.GeminiInlineData{MimeType: "image/jpeg", Data: "reference-two"}},
		}}},
	}
	request.GenerationConfig.CandidateCount = common.GetPointer(3)
	appendGeminiReferenceInstruction(request, 2)
	batch := &relaycommon.ImageBatchInfo{Mode: dto.ImageBatchModeFanout, RequestedCount: 3, ReferenceCount: 2}

	apiErr := runGeminiImageFanout(c, info, request, adaptor, batch)

	require.Nil(t, apiErr)
	assert.Equal(t, 3, batch.ResultCount)
	assert.Zero(t, batch.FailedCount)
	assert.Len(t, adaptor.requestBodies, 2)
	for _, body := range adaptor.requestBodies {
		var sent dto.GeminiChatRequest
		require.NoError(t, common.Unmarshal(body, &sent))
		require.NotNil(t, sent.GenerationConfig.CandidateCount)
		assert.Equal(t, 1, *sent.GenerationConfig.CandidateCount)
		require.Len(t, sent.Contents[0].Parts, 3)
		assert.Contains(t, sent.Contents[0].Parts[0].Text, "upload order")
		assert.Equal(t, "reference-one", sent.Contents[0].Parts[1].InlineData.Data)
		assert.Equal(t, "reference-two", sent.Contents[0].Parts[2].InlineData.Data)
	}
	var response dto.GeminiChatResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, 3, geminiResponseImageCount(&response))
	result, ok := GetCreationImageExecutionResult(c)
	require.True(t, ok)
	assert.Equal(t, 5, result.Usage.PromptTokens)
}

func TestCreationImageReferenceSemanticsUseMappedModelWhitelist(t *testing.T) {
	tests := []struct {
		model        string
		wantFidelity bool
	}{
		{model: "gpt-image-1", wantFidelity: true},
		{model: "gpt-image-1.5", wantFidelity: true},
		{model: "gpt-image-2", wantFidelity: true},
		{model: "gpt-image-1-mini", wantFidelity: false},
		{model: "chatgpt-image-latest", wantFidelity: false},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			request := &dto.ImageRequest{Prompt: "original"}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.model}}
			applyCreationImageReferenceSemantics(request, info, &creationImageExecutionState{ReferenceCount: 2})
			assert.Contains(t, request.Prompt, "Use all reference images in their upload order")
			if test.wantFidelity {
				assert.True(t, bytes.Equal([]byte(`"high"`), request.InputFidelity))
			} else {
				assert.Empty(t, request.InputFidelity)
			}
		})
	}

	request := &dto.ImageRequest{Prompt: "unchanged"}
	applyCreationImageReferenceSemantics(request, newCreationImageTestRelayInfo(), &creationImageExecutionState{})
	assert.Equal(t, "unchanged", request.Prompt)
}

func geminiResponseImageCount(response *dto.GeminiChatResponse) int {
	count := 0
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				count++
			}
		}
	}
	return count
}
