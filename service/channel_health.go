package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/channel_health_setting"
)

const (
	ChannelHealthNormal      = "normal"
	ChannelHealthDelayed     = "delayed"
	ChannelHealthUnavailable = "unavailable"
	ChannelHealthUnknown     = "unknown"
)

type ChannelHealthView struct {
	ChannelID           int     `json:"channel_id"`
	ModelName           string  `json:"model"`
	EndpointType        string  `json:"endpoint_type,omitempty"`
	Status              string  `json:"status"`
	RequestCount        int64   `json:"request_count"`
	SuccessRate         float64 `json:"success_rate"`
	AverageLatencyMs    int64   `json:"average_latency_ms"`
	AverageTTFTMs       int64   `json:"average_ttft_ms"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	LastSampleAt        int64   `json:"last_sample_at"`
	LastSuccessAt       int64   `json:"last_success_at"`
	LastErrorClass      string  `json:"last_error_class,omitempty"`
	LastErrorCode       string  `json:"last_error_code,omitempty"`
	LastHTTPStatus      int     `json:"last_http_status,omitempty"`
}

type PublicModelHealth struct {
	ModelName    string `json:"model"`
	Status       string `json:"status"`
	LastSampleAt int64  `json:"last_sample_at,omitempty"`
}

func GetChannelHealthViews(now time.Time) ([]ChannelHealthView, error) {
	setting := channel_health_setting.GetSetting()
	snapshots, err := model.GetChannelHealthSnapshots()
	if err != nil {
		return nil, err
	}
	windows, err := model.GetChannelHealthWindows(now.Add(-time.Duration(setting.WindowMinutes) * time.Minute).Unix())
	if err != nil {
		return nil, err
	}
	windowMap := make(map[string]model.ChannelHealthWindow, len(windows))
	for _, window := range windows {
		windowMap[channelHealthKey(window.ChannelID, window.ModelName)] = window
	}
	views := make([]ChannelHealthView, 0, len(snapshots))
	for _, snapshot := range snapshots {
		window := windowMap[channelHealthKey(snapshot.ChannelID, snapshot.ModelName)]
		views = append(views, EvaluateChannelHealth(now, snapshot, window))
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].ChannelID == views[j].ChannelID {
			return views[i].ModelName < views[j].ModelName
		}
		return views[i].ChannelID < views[j].ChannelID
	})
	return views, nil
}

func EvaluateChannelHealth(now time.Time, snapshot model.ChannelHealthSnapshot, window model.ChannelHealthWindow) ChannelHealthView {
	setting := channel_health_setting.GetSetting()
	view := ChannelHealthView{
		ChannelID:           snapshot.ChannelID,
		ModelName:           snapshot.ModelName,
		EndpointType:        snapshot.EndpointType,
		Status:              ChannelHealthUnknown,
		RequestCount:        window.RequestCount,
		ConsecutiveFailures: snapshot.ConsecutiveFailures,
		LastSampleAt:        snapshot.LastSampleAt,
		LastSuccessAt:       snapshot.LastSuccessAt,
		LastErrorClass:      snapshot.LastErrorClass,
		LastErrorCode:       snapshot.LastErrorCode,
		LastHTTPStatus:      snapshot.LastHTTPStatus,
	}
	if window.RequestCount > 0 {
		view.SuccessRate = float64(window.SuccessCount) * 100 / float64(window.RequestCount)
		latencyCount := window.LatencyCount
		if latencyCount <= 0 {
			// Rows written before latency_count was introduced remain readable.
			latencyCount = window.RequestCount - window.TTFTCount
		}
		if latencyCount > 0 {
			view.AverageLatencyMs = window.TotalLatencyMs / latencyCount
		}
	}
	if window.TTFTCount > 0 {
		view.AverageTTFTMs = window.TTFTSumMs / window.TTFTCount
	}
	if snapshot.LastSampleAt <= 0 || now.Unix()-snapshot.LastSampleAt >= int64(setting.StaleAfterMinutes*60) {
		return view
	}
	unavailableByRate := window.RequestCount >= int64(setting.MinimumSamples) &&
		window.FailureCount*100 >= window.RequestCount*int64(setting.UnavailableErrorRatePercent)
	if snapshot.ConsecutiveFailures >= setting.FailureStreakThreshold || unavailableByRate {
		view.Status = ChannelHealthUnavailable
		return view
	}
	if view.AverageLatencyMs >= int64(setting.DelayedThresholdMilliseconds) ||
		view.AverageTTFTMs >= int64(setting.DelayedThresholdMilliseconds) {
		view.Status = ChannelHealthDelayed
		return view
	}
	view.Status = ChannelHealthNormal
	return view
}

func GetPublicModelHealth(now time.Time) ([]PublicModelHealth, error) {
	views, err := GetChannelHealthViews(now)
	if err != nil {
		return nil, err
	}
	activeCounts, err := model.GetEnabledChannelModelCounts()
	if err != nil {
		return nil, err
	}
	aggregated := make(map[string]PublicModelHealth)
	for _, pricing := range model.GetPricing() {
		aggregated[pricing.ModelName] = PublicModelHealth{
			ModelName: pricing.ModelName,
			Status:    ChannelHealthUnknown,
		}
	}
	seen := make(map[string]int)
	unknown := make(map[string]bool)
	for _, view := range views {
		if activeCounts[view.ModelName] == 0 {
			continue
		}
		seen[view.ModelName]++
		current, exists := aggregated[view.ModelName]
		if !exists {
			current = PublicModelHealth{ModelName: view.ModelName, Status: ChannelHealthUnknown}
		}
		if view.Status == ChannelHealthUnknown {
			unknown[view.ModelName] = true
		}
		if view.Status == ChannelHealthNormal {
			current.Status = ChannelHealthNormal
		} else if current.Status != ChannelHealthNormal && view.Status == ChannelHealthDelayed {
			current.Status = ChannelHealthDelayed
		} else if current.Status == ChannelHealthUnknown && view.Status == ChannelHealthUnavailable {
			current.Status = ChannelHealthUnavailable
		}
		if view.LastSampleAt > current.LastSampleAt {
			current.LastSampleAt = view.LastSampleAt
		}
		aggregated[view.ModelName] = current
	}
	for modelName, count := range activeCounts {
		current, exists := aggregated[modelName]
		if !exists {
			current = PublicModelHealth{ModelName: modelName, Status: ChannelHealthUnknown}
		}
		if current.Status != ChannelHealthNormal && (count > seen[modelName] || unknown[modelName]) {
			current.Status = ChannelHealthUnknown
		} else if current.Status == ChannelHealthUnavailable && count != seen[modelName] {
			current.Status = ChannelHealthUnknown
		}
		aggregated[modelName] = current
	}
	result := make([]PublicModelHealth, 0, len(aggregated))
	for _, health := range aggregated {
		result = append(result, health)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ModelName < result[j].ModelName
	})
	return result, nil
}

func channelHealthKey(channelID int, modelName string) string {
	return fmt.Sprintf("%d:%s", channelID, modelName)
}
