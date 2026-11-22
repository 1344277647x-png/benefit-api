package creation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssetRetentionIsCappedAtThreeDays(t *testing.T) {
	previous := creationSetting
	t.Cleanup(func() { creationSetting = previous })
	for _, tc := range []struct{ configured, effective int }{
		{3, 3}, {7, 3}, {30, 3}, {1, 1}, {0, 1},
	} {
		creationSetting.RetentionDays = tc.configured
		assert.Equal(t, tc.effective, GetSetting().RetentionDays)
	}
}
