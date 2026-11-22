package xai

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestXAIStreamErrorIsDecodedAndClassified(t *testing.T) {
	var response dto.ChatCompletionsStreamResponse
	err := dto.GetOpenAIError(map[string]any{
		"message": "insufficient_quota",
		"code":    "insufficient_quota",
	})
	response.Error = map[string]any{
		"message": "insufficient_quota",
		"code":    "insufficient_quota",
	}
	decoded := response.GetOpenAIError()
	require.NotNil(t, decoded)
	require.Equal(t, "insufficient_quota", decoded.Message)
	classified := types.WithOpenAIError(*err, 502)
	require.Equal(t, types.ErrorClassUpstreamQuotaExhausted, classified.GetErrorClass())
}
