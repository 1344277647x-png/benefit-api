/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateResponsesStreamProtocol(t *testing.T) {
	t.Run("interleaved output items match their own ID and index", func(t *testing.T) {
		events := []string{
			`{"type":"response.created"}`,
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"A"}}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"id":"B"}}`,
			`{"type":"response.output_text.delta","output_index":1,"item_id":"B","delta":"b"}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"A"}}`,
			`{"type":"response.output_text.delta","output_index":1,"item_id":"B","delta":"c"}`,
			`{"type":"response.output_item.done","output_index":1,"item":{"id":"B"}}`,
			`{"type":"response.completed"}`,
		}
		check := func(events []string) *ChannelProtocolCheckReport {
			return validateResponsesStreamProtocol([]byte("data: " + strings.Join(events, "\n\ndata: ") + "\n\n"))
		}
		assert.True(t, check(events).Passed)
		events[2], events[3] = events[3], events[2]
		assert.False(t, check(events).Passed)
		events[2], events[3] = events[3], events[2]
		events[6], events[7] = events[7], events[6]
		assert.False(t, check(events).Passed)
	})
	t.Run("valid complete lifecycle passes", func(t *testing.T) {
		body := strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created","response":{"id":"resp_1"}}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message"}}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hello"}`,
			``,
			`event: response.output_item.done`,
			`data: {"type":"response.output_item.done","item":{"id":"msg_1"}}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
			``,
		}, "\n")
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckResponsesStream, []byte(body))
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.True(t, report.Passed)
		assert.Equal(t, channelProtocolCheckResponsesStream, report.Mode)
		assert.Contains(t, report.EventsSeen, "response.created")
		assert.Contains(t, report.EventsSeen, "response.completed")
	})

	t.Run("orphan output_text.delta without output_item.added fails", func(t *testing.T) {
		body := strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
			``,
			`event: response.completed`,
			`data: {"type":"response.completed"}`,
		}, "\n")
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckResponsesStream, []byte(body))
		require.Error(t, err)
		require.NotNil(t, report)
		assert.False(t, report.Passed)
		assert.Contains(t, report.Summary, "without an active output_item.added")
	})

	t.Run("truncated stream missing response.completed fails", func(t *testing.T) {
		body := strings.Join([]string{
			`event: response.created`,
			`data: {"type":"response.created"}`,
			``,
			`event: response.output_item.added`,
			`data: {"type":"response.output_item.added"}`,
			``,
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hi"}`,
		}, "\n")
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckResponsesStream, []byte(body))
		require.Error(t, err)
		require.NotNil(t, report)
		assert.False(t, report.Passed)
		assert.Contains(t, report.Summary, "Missing response.completed")
	})
}

func TestValidateImageEndpointProtocol(t *testing.T) {
	t.Run("valid PNG b64_json passes", func(t *testing.T) {
		pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0x0d, 'I', 'H', 'D', 'R'}
		payload := `{"created":1760000000,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(pngHeader) + `"}]}`
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckImageEndpoint, []byte(payload))
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.True(t, report.Passed)
		assert.Equal(t, "b64_json", report.ImageFormat)
	})

	t.Run("valid https url passes", func(t *testing.T) {
		payload := `{"created":1760000000,"data":[{"url":"https://cdn.example.com/out.png"}]}`
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckImageEndpoint, []byte(payload))
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.True(t, report.Passed)
		assert.Equal(t, "url", report.ImageFormat)
	})

	t.Run("upstream image model rejection fails", func(t *testing.T) {
		payload := `{"error":{"message":"images endpoint requires an image model","type":"invalid_request_error"}}`
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckImageEndpoint, []byte(payload))
		require.Error(t, err)
		require.NotNil(t, report)
		assert.False(t, report.Passed)
		assert.Contains(t, report.Summary, "images endpoint requires an image model")
	})

	t.Run("non-image base64 payload fails", func(t *testing.T) {
		payload := `{"created":1760000000,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("not-an-image-at-all")) + `"}]}`
		report, err := evaluateChannelProtocolCheck(channelProtocolCheckImageEndpoint, []byte(payload))
		require.Error(t, err)
		require.NotNil(t, report)
		assert.False(t, report.Passed)
		assert.Contains(t, report.Summary, "not a valid PNG/JPEG/GIF/WEBP image")
	})
}
