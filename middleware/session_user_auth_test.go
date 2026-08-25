package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSessionAwareUserAuthUsesRc25BearerAuthentication(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "session-aware-bearer-user", "session-aware-pat")

	router := gin.New()
	router.GET("/asset", SessionAwareUserAuth(), func(c *gin.Context) {
		require.Equal(t, user.Id, c.GetInt("id"))
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	request.Header.Set("Authorization", "Bearer session-aware-pat")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}
