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

func TestGetEndpointTypesIncludesGPTImageVersions(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-2", "4K gpt-image-2"} {
		t.Run(model, func(t *testing.T) {
			endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, model)
			assert.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
		})
	}
}

func TestIsGPTImageGenerationModelHandlesAliases(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "GPT-IMAGE-2", "  4K gpt-image-2  "} {
		assert.True(t, IsGPTImageGenerationModel(model), model)
	}
	assert.False(t, IsGPTImageGenerationModel("gpt-4o"))
}

func TestGetEndpointTypesRecognizesSeedreamAndNovelAIImageAliases(t *testing.T) {
	for _, model := range []string{
		"doubao-seedream-4-0-250828", "无限制-Seedream-5-Pro", "无限制-Seedream-5-Pro-2K",
		"无限制-NovelAI-V4.5", "无限制-NovelAI-V5", "nai-diffusion-4-5-full",
	} {
		t.Run(model, func(t *testing.T) {
			endpoints := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, model)
			assert.Contains(t, endpoints, constant.EndpointTypeImageGeneration)
		})
	}
	for _, model := range []string{"doubao-seed-1-6", "doubao-seedance-2-0", "novelai-text", "gpt-4o"} {
		t.Run(model, func(t *testing.T) {
			assert.NotContains(t, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, model), constant.EndpointTypeImageGeneration)
		})
	}
}
