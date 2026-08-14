package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreationErrorMessageExtractsStructuredError(t *testing.T) {
	message := creationErrorMessage(
		[]byte(`{"error":{"message":"no available channel for model gpt-image-2"}}`),
		"image generation failed",
	)
	require.Equal(t, "image generation failed: no available channel for model gpt-image-2", message)
}

func TestCreationErrorMessageFallsBackToRawBody(t *testing.T) {
	message := creationErrorMessage([]byte("upstream unavailable"), "image generation failed")
	require.Equal(t, "image generation failed: upstream unavailable", message)
}
