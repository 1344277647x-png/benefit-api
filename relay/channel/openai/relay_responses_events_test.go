package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamPreservesLifecycleReasoningToolsAndExtensionEvents(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	events := []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[]}}`,
		`{"type":"response.reasoning_summary_part.added","sequence_number":3,"item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":4,"item_id":"rs_1","output_index":0,"summary_index":0,"delta":"summary"}`,
		`{"type":"response.reasoning_summary_text.done","sequence_number":5,"item_id":"rs_1","output_index":0,"summary_index":0,"text":"summary"}`,
		`{"type":"response.reasoning_summary_part.done","sequence_number":6,"item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":"summary"}}`,
		`{"type":"response.output_item.done","sequence_number":7,"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"summary"}]}}`,
		`{"type":"response.output_item.added","sequence_number":8,"output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":9,"item_id":"fc_1","output_index":1,"delta":"{}"}`,
		`{"type":"response.function_call_arguments.done","sequence_number":10,"item_id":"fc_1","output_index":1,"arguments":"{}"}`,
		`{"type":"response.output_item.done","sequence_number":11,"output_index":1,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}}`,
		`{"type":"response.output_item.added","sequence_number":12,"output_index":2,"item":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`,
		`{"type":"response.content_part.added","sequence_number":13,"item_id":"msg_1","output_index":2,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":14,"item_id":"msg_1","output_index":2,"content_index":0,"delta":"hello"}`,
		`{"type":"response.output_text.annotation.added","sequence_number":15,"item_id":"msg_1","output_index":2,"content_index":0,"annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.org","title":"example"}}`,
		`{"type":"response.output_text.done","sequence_number":16,"item_id":"msg_1","output_index":2,"content_index":0,"text":"hello"}`,
		`{"type":"response.content_part.done","sequence_number":17,"item_id":"msg_1","output_index":2,"content_index":0,"part":{"type":"output_text","text":"hello"}}`,
		`{"type":"response.output_item.done","sequence_number":18,"output_index":2,"item":{"id":"msg_1","type":"message","status":"completed","content":[{"type":"output_text","text":"hello"}]}}`,
		`{"type":"response.future_extension","sequence_number":19,"extension":{"opaque_field":[1,2,3]}}`,
		`{"type":"response.completed","sequence_number":20,"response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
	}
	var upstream, expected strings.Builder
	for _, event := range events {
		var header struct {
			Type string `json:"type"`
		}
		require.NoError(t, common.UnmarshalJsonStr(event, &header))
		upstream.WriteString("data: " + event + "\n\n")
		expected.WriteString("event: " + header.Type + "\ndata: " + event + "\n\n")
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
	usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(upstream.String()))})
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, expected.String(), w.Body.String(), "every event and original JSON field must survive exactly once in upstream order")
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestResponsesPassthroughDoesNotForwardFailureBodiesOrInvalidEventHeaders(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, event := range []string{
		`{"type":"response.failed","response":{"error":{"code":"insufficient_quota","message":"synthetic-provider-secret"}}}`,
		`{"type":"response.incomplete","response":{"status":"incomplete","private_detail":"synthetic-provider-secret"}}`,
		`{"type":"response.future_extension","error":{"code":"insufficient_quota","message":"synthetic-provider-secret"}}`,
		`{"type":"response.future_extension\n\nevent: injected","detail":"synthetic-provider-secret"}`,
		`{"type":"","detail":"synthetic-provider-secret"}`,
	} {
		t.Run(event, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("data: " + event + "\n\n"))})
			assert.Nil(t, usage)
			require.NotNil(t, err)
			assert.Empty(t, w.Body.String())
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}
