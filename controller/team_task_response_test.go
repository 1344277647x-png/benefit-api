package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTeamTaskResponseWaitsForCommit(t *testing.T) {
	underlying := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(underlying)
	original := c.Writer
	pending := newTeamTaskResponseWriter(original)
	c.Writer = pending
	c.Header("X-New-Api-Other-Ratios", "{}")
	c.JSON(http.StatusAccepted, gin.H{"id": "task_test"})
	if underlying.Body.Len() != 0 || underlying.Header().Get("X-New-Api-Other-Ratios") != "" {
		t.Fatal("response was visible before task persistence")
	}
	c.Writer = original
	if err := pending.commit(); err != nil {
		t.Fatal(err)
	}
	if underlying.Code != http.StatusAccepted || underlying.Header().Get("X-New-Api-Other-Ratios") != "{}" || !strings.Contains(underlying.Body.String(), "task_test") {
		t.Fatalf("committed response missing: status=%d body=%q", underlying.Code, underlying.Body.String())
	}
}

func TestTeamTaskResponseDiscardAndLimit(t *testing.T) {
	underlying := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(underlying)
	pending := newTeamTaskResponseWriter(c.Writer)
	pending.Header().Set("X-Leak", "success")
	_, _ = pending.Write([]byte("upstream task id"))
	if underlying.Body.Len() != 0 || underlying.Header().Get("X-Leak") != "" {
		t.Fatal("discarded response leaked")
	}
	if _, err := pending.Write(make([]byte, maxTeamTaskResponseBytes)); err == nil || pending.err == nil {
		t.Fatal("oversized response accepted")
	}
}
