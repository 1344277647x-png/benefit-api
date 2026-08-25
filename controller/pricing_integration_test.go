package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasUsableIntegrationGroupRequiresUserAndPricingAccess(t *testing.T) {
	require.True(t, hasUsableIntegrationGroup(
		[]string{"vip"},
		[]string{"vip"},
		map[string]string{"vip": "VIP"},
	))
	require.False(t, hasUsableIntegrationGroup(
		[]string{"vip"},
		[]string{"default"},
		map[string]string{"vip": "VIP"},
	))
}

func TestHasUsableIntegrationGroupHonorsAllPricingGroup(t *testing.T) {
	require.True(t, hasUsableIntegrationGroup(
		[]string{"default"},
		[]string{"all"},
		map[string]string{"default": "Default"},
	))
}
