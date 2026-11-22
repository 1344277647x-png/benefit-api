package baidu

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestNewBaiduUpstreamErrorClassifiesQuota(t *testing.T) {
	err := newBaiduUpstreamError(BaiduChatResponse{
		Error: Error{
			ErrorCode: 17,
			ErrorMsg:  "account balance insufficient",
		},
	}, http.StatusBadGateway)

	require.Equal(t, types.ErrorClassUpstreamQuotaExhausted, err.GetErrorClass())
	require.Equal(t, types.UpstreamQuotaPublicMessage, err.Error())
	require.True(t, types.IsSkipRetryError(err))
}
