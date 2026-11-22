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
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/tidwall/gjson"
)

const (
	channelProtocolCheckNone            = ""
	channelProtocolCheckResponsesStream = "responses_stream"
	channelProtocolCheckImageEndpoint   = "image_endpoint"
	maxProtocolCheckStreamBytes         = 256 << 10
	maxProtocolCheckEventsRecorded      = 32
)

type ChannelProtocolCheckItem struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type ChannelProtocolCheckReport struct {
	Mode        string                     `json:"mode"`
	Passed      bool                       `json:"passed"`
	ImageFormat string                     `json:"image_format,omitempty"`
	EventsSeen  []string                   `json:"events_seen,omitempty"`
	Checks      []ChannelProtocolCheckItem `json:"checks"`
	Summary     string                     `json:"summary"`
}

func normalizeChannelProtocolCheck(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return channelProtocolCheckNone, nil
	case channelProtocolCheckResponsesStream:
		return channelProtocolCheckResponsesStream, nil
	case channelProtocolCheckImageEndpoint:
		return channelProtocolCheckImageEndpoint, nil
	default:
		return "", fmt.Errorf("unsupported protocol_check mode %q", raw)
	}
}

func applyProtocolCheckOverrides(mode string, endpointType string, isStream bool) (string, bool) {
	switch mode {
	case channelProtocolCheckResponsesStream:
		return string(constant.EndpointTypeOpenAIResponse), true
	case channelProtocolCheckImageEndpoint:
		return string(constant.EndpointTypeImageGeneration), false
	default:
		return endpointType, isStream
	}
}

func evaluateChannelProtocolCheck(mode string, respBody []byte) (*ChannelProtocolCheckReport, error) {
	switch mode {
	case channelProtocolCheckResponsesStream:
		report := validateResponsesStreamProtocol(respBody)
		if !report.Passed {
			return report, errors.New(report.Summary)
		}
		return report, nil
	case channelProtocolCheckImageEndpoint:
		report := validateImageEndpointProtocol(respBody)
		if !report.Passed {
			return report, errors.New(report.Summary)
		}
		return report, nil
	default:
		return nil, nil
	}
}

// validateResponsesStreamProtocol verifies OpenAI Responses SSE lifecycle
// completeness: response.created, no orphan output_text.delta without an active
// output_item.added, no terminal error/incomplete event, and response.completed.
func validateResponsesStreamProtocol(respBody []byte) *ChannelProtocolCheckReport {
	eventsSeen := make([]string, 0, 8)
	seenSet := make(map[string]struct{})
	recordEvent := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seenSet[name]; ok {
			return
		}
		seenSet[name] = struct{}{}
		if len(eventsSeen) < maxProtocolCheckEventsRecorded {
			eventsSeen = append(eventsSeen, name)
		}
	}

	var (
		hasAnyEvent         bool
		hasCreated          bool
		hasCompleted        bool
		orphanDeltaCount    int
		invalidLifecycle    bool
		terminalFailureName string
		currentEventHeader  string
	)
	activeByID := map[string]*gjson.Result{}
	activeByIndex := map[int64]string{}

	for _, rawLine := range bytes.Split(respBody, []byte{'\n'}) {
		line := strings.TrimSpace(string(rawLine))
		if line == "" {
			currentEventHeader = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEventHeader = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		eventType := currentEventHeader
		if gjson.Valid(payload) {
			if t := strings.TrimSpace(gjson.Get(payload, "type").String()); t != "" {
				eventType = t
			}
			if msg := detectErrorMessageFromJSONBytes([]byte(payload)); msg != "" && terminalFailureName == "" {
				terminalFailureName = "error: " + msg
			}
		} else {
			invalidLifecycle = true
			continue
		}
		if eventType == "" {
			continue
		}
		hasAnyEvent = true
		recordEvent(eventType)

		itemID := gjson.Get(payload, "item_id").String()
		if itemID == "" {
			itemID = gjson.Get(payload, "item.id").String()
		}
		index := gjson.Get(payload, "output_index")
		if index.Exists() && (index.Type != gjson.Number || index.Num < 0 || index.Num != float64(index.Int())) {
			invalidLifecycle = true
			continue
		}
		if itemID == "" && index.Exists() {
			itemID = activeByIndex[index.Int()]
		}
		activeIndex, added := activeByID[itemID]
		active := added
		if active && index.Exists() && (activeIndex == nil || activeIndex.Int() != index.Int()) {
			active = false
		}
		if hasCompleted || (!hasCreated && strings.HasPrefix(eventType, "response.") && eventType != "response.created") {
			invalidLifecycle = true
		}
		switch eventType {
		case "response.created":
			invalidLifecycle = invalidLifecycle || hasCreated
			hasCreated = true
		case "response.output_item.added":
			if itemID == "" || added || (index.Exists() && activeByIndex[index.Int()] != "") {
				invalidLifecycle = true
				continue
			}
			activeByID[itemID] = nil
			if index.Exists() {
				activeByID[itemID] = &index
				activeByIndex[index.Int()] = itemID
			}
		case "response.output_item.done":
			if !active {
				invalidLifecycle = true
				continue
			}
			delete(activeByID, itemID)
			if activeIndex != nil {
				delete(activeByIndex, activeIndex.Int())
			}
		case "response.output_text.delta":
			if !active {
				orphanDeltaCount++
			}
		case "response.completed":
			invalidLifecycle = invalidLifecycle || len(activeByID) > 0
			hasCompleted = true
		case "response.failed", "response.incomplete", "error":
			if terminalFailureName == "" {
				terminalFailureName = eventType
			}
		}
	}

	checks := []ChannelProtocolCheckItem{
		{
			Name:   "sse_events_present",
			Passed: hasAnyEvent,
			Detail: boolDetail(hasAnyEvent, "Parsed valid Responses SSE events", "No Responses SSE events parsed from stream"),
		},
		{
			Name:   "response_created",
			Passed: hasCreated,
			Detail: boolDetail(hasCreated, "Observed response.created", "Missing response.created lifecycle event"),
		},
		{
			Name:   "output_item_lifecycle",
			Passed: orphanDeltaCount == 0,
			Detail: boolDetail(orphanDeltaCount == 0, "All output_text.delta events had an active output item", fmt.Sprintf("Observed %d output_text.delta event(s) without an active output_item.added", orphanDeltaCount)),
		},
		{
			Name:   "response_event_order",
			Passed: !invalidLifecycle,
			Detail: boolDetail(!invalidLifecycle, "Response and output items follow lifecycle order", "Invalid response lifecycle order or output item identity"),
		},
		{
			Name:   "no_terminal_error",
			Passed: terminalFailureName == "",
			Detail: boolDetail(terminalFailureName == "", "No error/failed/incomplete terminal event", "Stream ended with terminal failure: "+terminalFailureName),
		},
		{
			Name:   "response_completed",
			Passed: hasCompleted,
			Detail: boolDetail(hasCompleted, "Observed response.completed", "Missing response.completed terminal event (stream truncated or incompatible)"),
		},
	}

	passed := true
	failures := make([]string, 0)
	for _, item := range checks {
		if !item.Passed {
			passed = false
			failures = append(failures, item.Detail)
		}
	}
	summary := "Responses stream lifecycle check passed"
	if !passed {
		summary = "Responses stream protocol check failed: " + strings.Join(failures, "; ")
	}
	return &ChannelProtocolCheckReport{
		Mode:       channelProtocolCheckResponsesStream,
		Passed:     passed,
		EventsSeen: eventsSeen,
		Checks:     checks,
		Summary:    summary,
	}
}

func hasKnownImageMagic(raw []byte) bool {
	if len(raw) < 12 {
		return false
	}
	// PNG
	if bytes.HasPrefix(raw, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return true
	}
	// JPEG
	if len(raw) >= 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF {
		return true
	}
	// GIF
	if bytes.HasPrefix(raw, []byte("GIF87a")) || bytes.HasPrefix(raw, []byte("GIF89a")) {
		return true
	}
	// WEBP
	if bytes.HasPrefix(raw, []byte("RIFF")) && bytes.Equal(raw[8:12], []byte("WEBP")) {
		return true
	}
	return false
}

// validateImageEndpointProtocol checks OpenAI-compatible /v1/images/generations
// responses for JSON integrity, absence of upstream errors, non-empty data list,
// and valid image URL or decoded image base64 magic bytes.
func validateImageEndpointProtocol(respBody []byte) *ChannelProtocolCheckReport {
	trimmed := bytes.TrimSpace(respBody)
	validJSON := len(trimmed) > 0 && gjson.ValidBytes(trimmed)
	upstreamErr := ""
	if validJSON {
		upstreamErr = detectErrorMessageFromJSONBytes(trimmed)
	}

	dataArr := gjson.GetBytes(trimmed, "data")
	hasData := validJSON && dataArr.IsArray() && len(dataArr.Array()) > 0
	imageFormat := ""
	artifactValid := false
	artifactDetail := "First data item lacks a valid image url or decodable b64_json payload"

	if hasData {
		first := dataArr.Array()[0]
		rawURL := strings.TrimSpace(first.Get("url").String())
		rawB64 := strings.TrimSpace(first.Get("b64_json").String())
		if rawB64 != "" {
			imageFormat = "b64_json"
			// Strip optional data URI prefix before decoding magic header.
			if idx := strings.Index(rawB64, ","); strings.HasPrefix(rawB64, "data:") && idx > 0 {
				rawB64 = rawB64[idx+1:]
			}
			sample := rawB64
			if len(sample) > 64 {
				sample = sample[:64]
				sample = sample[:len(sample)-(len(sample)%4)]
			}
			decoded, err := base64.StdEncoding.DecodeString(sample)
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(sample, "="))
			}
			if err == nil && hasKnownImageMagic(decoded) {
				artifactValid = true
				artifactDetail = "Verified image magic bytes in data[0].b64_json"
			} else {
				artifactDetail = "data[0].b64_json is not a valid PNG/JPEG/GIF/WEBP image"
			}
		} else if rawURL != "" {
			imageFormat = "url"
			if parsed, err := url.Parse(rawURL); err == nil &&
				(strings.EqualFold(parsed.Scheme, "https") || strings.EqualFold(parsed.Scheme, "http")) &&
				parsed.Hostname() != "" {
				artifactValid = true
				artifactDetail = "Verified HTTP(S) image URL in data[0].url"
			} else {
				artifactDetail = "data[0].url is not a valid HTTP(S) URL"
			}
		}
	}

	checks := []ChannelProtocolCheckItem{
		{
			Name:   "json_payload",
			Passed: validJSON,
			Detail: boolDetail(validJSON, "Response body is valid JSON", "Response body is empty or not valid JSON"),
		},
		{
			Name:   "no_upstream_error",
			Passed: upstreamErr == "",
			Detail: boolDetail(upstreamErr == "", "No upstream error object returned", "Upstream error: "+upstreamErr),
		},
		{
			Name:   "image_data_present",
			Passed: hasData,
			Detail: boolDetail(hasData, "Response contains non-empty data[] array", "Response missing non-empty data[] image array"),
		},
		{
			Name:   "image_artifact_valid",
			Passed: artifactValid,
			Detail: artifactDetail,
		},
	}

	passed := true
	failures := make([]string, 0)
	for _, item := range checks {
		if !item.Passed {
			passed = false
			failures = append(failures, item.Detail)
		}
	}
	summary := "Image endpoint compatibility check passed"
	if !passed {
		summary = "Image endpoint compatibility check failed: " + strings.Join(failures, "; ")
	}
	return &ChannelProtocolCheckReport{
		Mode:        channelProtocolCheckImageEndpoint,
		Passed:      passed,
		ImageFormat: imageFormat,
		Checks:      checks,
		Summary:     summary,
	}
}

func boolDetail(ok bool, passText string, failText string) string {
	if ok {
		return passText
	}
	return failText
}
