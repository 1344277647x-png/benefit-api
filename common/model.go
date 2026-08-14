package common

import "strings"

var (
	// OpenAIResponseOnlyModels is a list of models that are only available for OpenAI responses.
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}
	ImageGenerationModels = []string{
		"dall-e-3",
		"dall-e-2",
		"gpt-image-",
		"prefix:imagen-",
		"flux-",
		"flux.1-",
	}
	OpenAITextModels = []string{
		"gpt-",
		"o1",
		"o3",
		"o4",
		"chatgpt",
	}
)

func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

func IsImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	if IsGPTImageGenerationModel(modelName) {
		return true
	}
	for _, m := range ImageGenerationModels {
		if m == "gpt-image-" {
			continue
		}
		if strings.Contains(modelName, m) {
			return true
		}
		if strings.HasPrefix(m, "prefix:") && strings.HasPrefix(modelName, strings.TrimPrefix(m, "prefix:")) {
			return true
		}
	}
	return false
}

// IsGPTImageGenerationModel recognizes the whole GPT Image family, including
// provider aliases such as "4K gpt-image-2".
func IsGPTImageGenerationModel(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "gpt-image-")
}

func IsGeminiImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if !strings.Contains(modelName, "gemini") {
		return false
	}
	return strings.Contains(modelName, "image") || strings.Contains(modelName, "banana")
}

func IsVideoGenerationModel(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	for _, marker := range []string{"veo-", "seedance-", "sora-"} {
		if strings.Contains(modelName, marker) {
			return true
		}
	}
	return false
}

func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
