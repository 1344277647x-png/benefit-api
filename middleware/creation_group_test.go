package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func creationGroupTestContext() *gin.Context {
	context, _ := gin.CreateTestContext(nil)
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "default")
	return context
}

func TestSelectCreationGroupDefaultsToAccountGroup(t *testing.T) {
	context := creationGroupTestContext()
	group, err := selectCreationGroup(context, "")

	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.Equal(t, "default", common.GetContextKeyString(context, constant.ContextKeyUsingGroup))
}

func TestSelectCreationGroupAllowsAuthorizedNonDefaultGroup(t *testing.T) {
	context := creationGroupTestContext()
	group, err := selectCreationGroup(context, "vip")

	require.NoError(t, err)
	require.Equal(t, "vip", group)
	require.Equal(t, "vip", common.GetContextKeyString(context, constant.ContextKeyUsingGroup))
}

func TestSelectCreationGroupRejectsUnauthorizedGroup(t *testing.T) {
	context := creationGroupTestContext()
	_, err := selectCreationGroup(context, "svip")

	require.ErrorContains(t, err, "no access")
	require.Equal(t, "default", common.GetContextKeyString(context, constant.ContextKeyUsingGroup))
}
