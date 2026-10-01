package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiChatHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiStreamHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiStreamHandlerMapsResourceExhaustedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`data: {"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}` + "\n\n",
		)),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, apiErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorClassUpstreamQuotaExhausted, apiErr.GetErrorClass())
	assert.Equal(t, types.UpstreamQuotaPublicMessage, apiErr.ToOpenAIError().Message)
	assert.NotContains(t, recorder.Body.String(), "Resource exhausted")
}

func TestGeminiNonStreamHandlersRecognizeTopLevelQuotaError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"error":{"message":"account balance insufficient","code":402,"status":"FAILED_PRECONDITION"}}`
	for _, tc := range []struct {
		name string
		hand func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "native", hand: GeminiTextGenerationHandler},
		{name: "responses", hand: GeminiResponsesHandler},
		{name: "image", hand: GeminiImageHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
			resp := &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := tc.hand(c, info, resp)
			require.Nil(t, usage)
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorClassUpstreamQuotaExhausted, apiErr.GetErrorClass())
			assert.Equal(t, types.UpstreamQuotaPublicMessage, apiErr.ToOpenAIError().Message)
		})
	}
}

func TestGeminiChatHandlerDoesNotBillBlockedEmptyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	blockReason := "SAFETY"
	body, err := common.Marshal(dto.GeminiChatResponse{
		PromptFeedback: &dto.GeminiChatPromptFeedback{BlockReason: &blockReason},
		UsageMetadata:  dto.GeminiUsageMetadata{PromptTokenCount: 12, TotalTokenCount: 12},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}

	usage, apiErr := GeminiChatHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodePromptBlocked, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Empty(t, recorder.Body.String(), "the controller owns the only client error response")
}

func TestGeminiTextGenerationHandlerDoesNotBillBlockedEmptyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	blockReason := "SAFETY"
	body, err := common.Marshal(dto.GeminiChatResponse{
		PromptFeedback: &dto.GeminiChatPromptFeedback{BlockReason: &blockReason},
		UsageMetadata:  dto.GeminiUsageMetadata{PromptTokenCount: 12, TotalTokenCount: 12},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-test:generateContent", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}

	usage, apiErr := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodePromptBlocked, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Empty(t, recorder.Body.String())
}

func TestGeminiStreamHandlerRejectsEOFWithoutDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}` + "\n\n")), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	usage, apiErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeBadResponse, apiErr.GetErrorCode())
}

func TestGeminiTextGenerationHandlerPromptTokensIncludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiChatHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiStreamHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiTextGenerationHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiChatHandlerMissingUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	body := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`)
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.Equal(t, dto.BillingUsageSemanticGemini, usage.BillingUsage.Semantic)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}

func TestGeminiStreamHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	// Simulates a client aborting the stream before the final chunk: text was
	// streamed but the last observed usageMetadata only carries prompt tokens.
	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial streamed answer before disconnect"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
}

func TestGeminiChatHandlerPromptOnlyUsageMetadataEstimatesCompletionTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "answer text without candidate token count"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount: 151,
			TotalTokenCount:  151,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 151, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
}

func TestGeminiStreamHandlerEmptyUsageMetadataBuildsEstimatedBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	streamBody := []byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]}}],\"usageMetadata\":{}}\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.NotNil(t, usage.BillingUsage)
	require.True(t, usage.BillingUsage.Estimated)
	require.Equal(t, dto.BillingUsageSourceGeminiChat, usage.BillingUsage.Source)
	require.NotNil(t, usage.BillingUsage.GeminiUsageMetadata)
	require.Equal(t, usage.PromptTokens, usage.BillingUsage.GeminiUsageMetadata.PromptTokenCount)
	require.Equal(t, usage.CompletionTokens, usage.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
}
