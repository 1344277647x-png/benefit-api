package dify

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestNewDifyUpstreamErrorClassifiesQuota(t *testing.T) {
	err := newDifyUpstreamError("quota_exceeded", "credits exhausted", http.StatusBadGateway)

	require.Equal(t, types.ErrorClassUpstreamQuotaExhausted, err.GetErrorClass())
	require.Equal(t, types.UpstreamQuotaPublicMessage, err.Error())
	require.True(t, types.IsSkipRetryError(err))
}

func TestFirstNonEmptyUsesProviderMessage(t *testing.T) {
	require.Equal(t, "provider message", firstNonEmpty("provider message", "fallback"))
	require.Equal(t, "fallback", firstNonEmpty("", "fallback"))
}
