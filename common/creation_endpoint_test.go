package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestGetEndpointTypesIncludesNativeGeminiImageCapability(t *testing.T) {
	endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeGemini, "gemini-3-pro-image-preview")
	assert.Contains(t, endpoints, constant.EndpointTypeGeminiImage)
	assert.Contains(t, endpoints, constant.EndpointTypeGemini)
	assert.NotContains(t, endpoints, constant.EndpointTypeImageGeneration)
}

func TestGetEndpointTypesIncludesVideoCapability(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		model       string
	}{
		{name: "veo", channelType: constant.ChannelTypeGemini, model: "veo-3.1-generate-preview"},
		{name: "seedance", channelType: constant.ChannelTypeOpenAI, model: "seedance-2.5"},
		{name: "sora", channelType: constant.ChannelTypeSora, model: "sora-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			endpoints := GetEndpointTypesByChannelType(test.channelType, test.model)
			assert.Contains(t, endpoints, constant.EndpointTypeOpenAIVideo)
		})
	}
}
