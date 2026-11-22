package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyUpstreamErrorRequiresExplicitQuotaSignal(t *testing.T) {
	require.Equal(t, ErrorClassUpstreamQuotaExhausted,
		ClassifyUpstreamError("insufficient_quota", "", "", http.StatusPaymentRequired))
	require.Equal(t, ErrorClassUpstreamRateLimited,
		ClassifyUpstreamError("rate_limit_exceeded", "rate_limit_error", "try later", http.StatusTooManyRequests))
	require.NotEqual(t, ErrorClassUpstreamQuotaExhausted,
		ClassifyUpstreamError("rate_limit_exceeded", "rate_limit_error", "try later", http.StatusTooManyRequests))
}

func TestClassifyUpstreamErrorRecognizesProviderQuotaSignals(t *testing.T) {
	for _, signal := range []string{
		"quota_exceeded", "billing_limit_reached", "billing_hard_limit_reached",
		"You exceeded your current quota, please check your plan and billing details.",
		"insufficient credits", "credits exhausted",
		"account balance insufficient", "余额不足",
	} {
		t.Run(signal, func(t *testing.T) {
			require.Equal(t, ErrorClassUpstreamQuotaExhausted,
				ClassifyUpstreamError("provider_error", "upstream_error", signal, http.StatusBadGateway))
		})
	}
}

func TestClassifyUpstreamErrorDoesNotInferQuotaFromStatusAlone(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			require.NotEqual(t, ErrorClassUpstreamQuotaExhausted,
				ClassifyUpstreamError("provider_error", "upstream_error", "request failed", status))
		})
	}
}

func TestWithOpenAIErrorReplacesAllPublicQuotaRepresentations(t *testing.T) {
	err := WithOpenAIError(OpenAIError{
		Message: "insufficient_quota: credits exhausted",
		Type:    "invalid_request_error",
		Code:    "insufficient_quota",
	}, http.StatusPaymentRequired)

	require.Equal(t, ErrorClassUpstreamQuotaExhausted, err.GetErrorClass())
	require.Equal(t, ErrorCodeUpstreamQuotaExhausted, err.GetErrorCode())
	require.True(t, IsSkipRetryError(err), "exhausted provider quota must not enter channel failover")
	require.Equal(t, UpstreamQuotaPublicMessage, err.Error())
	require.Equal(t, UpstreamQuotaPublicMessage, err.ToOpenAIError().Message)
	require.NotContains(t, err.ToOpenAIError().Message, "credits exhausted")
}

func TestWithClaudeErrorSkipsRetryForProviderQuota(t *testing.T) {
	err := WithClaudeError(ClaudeError{
		Type:    "upstream_error",
		Message: "billing limit reached",
	}, http.StatusBadGateway)

	require.Equal(t, ErrorClassUpstreamQuotaExhausted, err.GetErrorClass())
	require.True(t, IsSkipRetryError(err))
	require.Equal(t, UpstreamQuotaPublicMessage, err.ToClaudeError().Message)
}

func TestPlatformQuotaRemainsDistinct(t *testing.T) {
	err := NewErrorWithStatusCode(errors.New("余额不足，请充值"), ErrorCodeInsufficientUserQuota, http.StatusForbidden)

	require.Equal(t, ErrorClassPlatformQuotaInsufficient, err.GetErrorClass())
	require.Equal(t, ErrorCodeInsufficientUserQuota, err.GetErrorCode())
	require.Contains(t, err.ToOpenAIError().Message, "余额不足")
}

func TestLocalErrorTextDoesNotBecomeUpstreamQuota(t *testing.T) {
	err := NewErrorWithStatusCode(errors.New("upstream diagnostic: account balance insufficient"), ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	require.NotEqual(t, ErrorClassUpstreamQuotaExhausted, err.GetErrorClass())
	require.NotEqual(t, ErrorCodeUpstreamQuotaExhausted, err.GetErrorCode())
}
