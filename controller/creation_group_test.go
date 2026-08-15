package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompatibleCreationGroupsIntersectsAndSortsAccountFirst(t *testing.T) {
	groups := compatibleCreationGroups(
		[]string{"vip", "default", "inactive", "vip"},
		[]string{"all"},
		map[string]string{
			"default":  "Default",
			"inactive": "Inactive",
			"vip":      "VIP",
		},
		"default",
	)

	require.Equal(t, []string{"default", "vip"}, groups)
}

func TestCompatibleCreationGroupsHonorsPricingGroups(t *testing.T) {
	groups := compatibleCreationGroups(
		[]string{"default", "vip", "svip"},
		[]string{"svip"},
		map[string]string{
			"default": "Default",
			"vip":     "VIP",
			"svip":    "SVIP",
		},
		"default",
	)

	require.Equal(t, []string{"svip"}, groups)
}
