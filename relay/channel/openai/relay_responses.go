package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/MoeForge/moego/ojson"
	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); dto.HasOpenAIError(oaiError) {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	info.ObserveResponseModel(responsesResponse.Model)
	responseBody = rewriteSGLangResponsesCreatedAt(info, responseBody, "created_at", responsesResponse.CreatedAt)

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}
	info.ApplyVendorToolUsage(responseBody)

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			imageCounter.Observe(&responsesResponse.Output[i], &i)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func responsesReportedStatus(response *dto.OpenAIResponsesResponse) (string, bool) {
	if response == nil {
		return "", false
	}
	// Some compatible providers omit status on a completed response.
	if len(response.Status) == 0 {
		return "", true
	}
	var status *string
	if common.Unmarshal(response.Status, &status) != nil || status == nil {
		return "", false
	}
	return *status, true
}

func validResponsesCompletion(response *dto.OpenAIResponsesResponse) bool {
	status, valid := responsesReportedStatus(response)
	return valid && (status == "" || status == "completed")
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (resultUsage *dto.Usage, resultErr *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, common.LogText("invalid response or response body"))
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)
	var streamErr *types.NewAPIError
	sawTerminal := false
	defer func() {
		if resultErr != nil {
			accumulator.CommitObservedImages()
			service.RetainPartialStreamUsage(c, info, accumulator.PartialUsage(), accumulator.GeneratedText())
		}
	}()

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, common.LogText("failed to unmarshal stream response: %s", err.Error()))
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		if streamResponse.Type == "" || strings.ContainsAny(streamResponse.Type, "\r\n\x00") {
			streamErr = types.NewOpenAIError(fmt.Errorf("invalid upstream Responses event type"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}
		response := streamResponse.Response
		if response != nil {
			data = string(rewriteSGLangResponsesCreatedAt(info, []byte(data), "response.created_at", response.CreatedAt))
		}
		status, validStatus := responsesReportedStatus(response)
		completion := streamResponse.Type == "response.completed" || streamResponse.Type == "response.done"
		terminal := completion
		malformed := completion && (!validStatus || (status != "" && status != "completed"))
		explicitFailure := false
		switch streamResponse.Type {
		case "response.incomplete", "response.cancelled", "response.canceled":
			terminal = true
			expected := strings.TrimPrefix(streamResponse.Type, "response.")
			malformed = !validStatus || status != expected
			if expected == "incomplete" && !malformed {
				malformed = response.IncompleteDetails == nil || response.IncompleteDetails.Reason == ""
			}
			explicitFailure = expected != "incomplete"
		case "response.failed", "response.error", "error":
			terminal, explicitFailure = true, true
		}
		upstreamErr := streamResponse.GetOpenAIError()
		if !dto.HasOpenAIError(upstreamErr) && response != nil {
			upstreamErr = response.GetOpenAIError()
		}
		// Observe the upstream outcome separately from delivery. Some providers
		// put a failed or incomplete response inside response.done.
		reportedFailure := completion && (status == "failed" || status == "incomplete" || status == "cancelled" || status == "canceled")
		if !malformed || reportedFailure {
			service.ObserveResponsesOutcome(info, &streamResponse)
			if response != nil {
				service.ApplyResponsesUsage(accumulator.PartialUsage(), response.Usage)
			}
		}
		// A rejected request can be retried before streaming has started. An
		// explicit terminal on an accepted response belongs to that response
		// and settles exactly once, like the Responses WebSocket transport.
		rejected := explicitFailure && !helper.StreamOutputWritten(c)
		if rejected || malformed || (!explicitFailure && dto.HasOpenAIError(upstreamErr)) {
			accumulator.DiscardObservedImages()
			err := fmt.Errorf("upstream Responses stream did not complete")
			if dto.HasOpenAIError(upstreamErr) {
				streamErr = types.WithOpenAIError(*upstreamErr, http.StatusBadGateway)
			} else {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
			}
			sr.Stop(err)
			return
		}
		if terminal && !completion {
			accumulator.DiscardObservedImages()
		}
		if err := helper.ResponseChunkData(c, streamResponse, data); err != nil {
			if ctxErr := c.Request.Context().Err(); ctxErr != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, ctxErr)
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
			sr.Stop(err)
			return
		}
		accumulator.Observe(&streamResponse, common.StringToByteSlice(data))
		if terminal {
			sawTerminal = true
			if value, ok := c.Get(relaycommon.StreamDiagnosticsContextKey); ok {
				if diagnostics, ok := value.(*relaycommon.StreamDiagnostics); ok && completion {
					diagnostics.Completed(time.Now())
				}
			}
			sr.Done()
		}
	})
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	if streamErr != nil {
		return nil, streamErr
	}
	if info.ReceivedResponseCount == 0 && info.StreamStatus != nil {
		reason := info.StreamStatus.EndReason
		if reason == relaycommon.StreamEndReasonTimeout ||
			reason == relaycommon.StreamEndReasonScannerErr ||
			reason == relaycommon.StreamEndReasonEOF {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("upstream Responses stream ended before the first event: %s", reason),
				types.ErrorCodeReadResponseBodyFailed,
				http.StatusBadGateway,
			)
		}
	}

	if !sawTerminal || info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone || info.StreamStatus.HasErrors() {
		return nil, types.NewOpenAIError(fmt.Errorf("upstream Responses stream ended without a completed event"), types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	return accumulator.Finish(), nil
}

func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, createdAt dto.IntValue) []byte {
	if info.GetChannelType() != constant.ChannelTypeSGLang {
		return payload
	}
	if !ojson.GetBytes(payload, path).Exists() {
		return payload
	}
	patched, err := ojson.SetBytes(payload, path, int(createdAt))
	if err != nil {
		return payload
	}
	return patched
}
