package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveImage2SizeMapsEveryResolutionAndAspectRatio(t *testing.T) {
	expected := map[string]map[string]string{
		"1K": {
			"1:1": "1024x1024", "3:2": "1536x1024", "2:3": "1024x1536", "16:9": "1280x720",
			"9:16": "720x1280", "4:3": "1024x768", "3:4": "768x1024", "21:9": "1280x544",
		},
		"2K": {
			"1:1": "2048x2048", "3:2": "2160x1440", "2:3": "1440x2160", "16:9": "2560x1440",
			"9:16": "1440x2560", "4:3": "2048x1536", "3:4": "1536x2048", "21:9": "2560x1088",
		},
		"4K": {
			"1:1": "2880x2880", "3:2": "3456x2304", "2:3": "2304x3456", "16:9": "3840x2160",
			"9:16": "2160x3840", "4:3": "3200x2400", "3:4": "2400x3200", "21:9": "3840x1600",
		},
	}

	for resolution, sizes := range expected {
		for aspectRatio, expectedSize := range sizes {
			actual, ok := ResolveImage2Size(resolution, aspectRatio)
			require.True(t, ok, "%s %s", resolution, aspectRatio)
			assert.Equal(t, expectedSize, actual, "%s %s", resolution, aspectRatio)
		}
	}
	_, ok := ResolveImage2Size("8K", "16:9")
	assert.False(t, ok)
	_, ok = ResolveImage2Size("4K", "4:1")
	assert.False(t, ok)
}

func TestIsImage2ModelUsesExactFamilyBoundary(t *testing.T) {
	assert.True(t, IsImage2Model("gpt-image-2"))
	assert.True(t, IsImage2Model("gpt-image-2.5-flare"))
	assert.True(t, IsImage2Model("gpt-image-2.5-sunburst"))
	assert.False(t, IsImage2Model("gpt-image-20"))
	assert.False(t, IsImage2Model("gpt-image-1"))
}
