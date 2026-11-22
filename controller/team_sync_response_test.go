package controller

import (
	"bufio"
	stdctx "context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trackingTeamHijackWriter struct {
	gin.ResponseWriter
	hijacks int
}

func (w *trackingTeamHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacks++
	return nil, nil, errors.New("underlying connection was exposed")
}

type cancelingTeamResponseWriter struct {
	gin.ResponseWriter
	cancel stdctx.CancelFunc
	writes int
}

func (w *cancelingTeamResponseWriter) Write(data []byte) (int, error) {
	w.writes++
	n, err := w.ResponseWriter.Write(data)
	w.cancel()
	return n, err
}

func TestTeamSyncResponseCrashCleanup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows delete-on-close contract")
	}
	if os.Getenv("TEAM_RESPONSE_CRASH_HELPER") == "1" {
		underlying := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(underlying)
		writer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
		if err != nil {
			os.Exit(2)
		}
		if _, err := writer.WriteString("private team result"); err != nil {
			os.Exit(3)
		}
		fmt.Print(writer.file.Name())
		os.Exit(0) // Simulates a terminated process without close or deferred cleanup.
	}
	t.Setenv("TEAM_RESPONSE_STAGING_DIR", filepath.Join(`D:\CodexTemp`, "team-response-test"))
	cmd := exec.Command(os.Args[0], "-test.run=^TestTeamSyncResponseCrashCleanup$")
	cmd.Env = append(os.Environ(), "TEAM_RESPONSE_CRASH_HELPER=1")
	output, err := cmd.Output()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	assert.Equal(t, filepath.Join(`D:\CodexTemp`, "team-response-test"), filepath.Dir(path))
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "temporary response should disappear on process termination")
}

func TestTeamSyncResponseStagingUsesDDriveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows drive policy")
	}
	t.Setenv("TEAM_RESPONSE_STAGING_DIR", `C:\team-response-staging`)
	underlying := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(underlying)
	_, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.ErrorContains(t, err, "D drive")
	t.Setenv("TEAM_RESPONSE_STAGING_DIR", filepath.Join(`D:\CodexTemp`, "team-response-test"))
	writer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	assert.True(t, strings.EqualFold("D:", filepath.VolumeName(writer.file.Name())))
	require.NoError(t, writer.close())
}

func TestTeamSyncResponseIsPrivateUntilCommitAndFlushCannotLeak(t *testing.T) {
	underlying := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(underlying)
	buffer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	path := buffer.file.Name()
	t.Cleanup(func() { _ = buffer.close() })
	context.Writer = buffer
	context.Header("X-Team-Result", "pending")
	context.Writer.WriteHeader(http.StatusOK)
	_, err = context.Writer.WriteString("data: team success\n\n")
	require.NoError(t, err)
	context.Writer.Flush()
	assert.Empty(t, underlying.Body.String())
	assert.Empty(t, underlying.Header().Get("X-Team-Result"))
	assert.True(t, buffer.Written())
	require.NoError(t, buffer.commit())
	assert.Equal(t, "data: team success\n\n", underlying.Body.String())
	assert.Equal(t, "pending", underlying.Header().Get("X-Team-Result"))
	require.NoError(t, buffer.close())
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "only this response file should be removed")
}

func TestTeamSyncResponseDiscardDoesNotLeakSuccessfulUpstreamBody(t *testing.T) {
	underlying := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(underlying)
	buffer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	context.Writer = buffer
	context.Header("X-Upstream", "success")
	_, err = context.Writer.WriteString(strings.Repeat("success", 20))
	require.NoError(t, err)
	context.Writer = buffer.ResponseWriter
	require.NoError(t, buffer.close())
	assert.Empty(t, underlying.Body.String())
	assert.Empty(t, underlying.Header().Get("X-Upstream"))
}

func TestTeamSyncResponseBoundsConcurrentStagingAndReleasesSlot(t *testing.T) {
	underlying := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(underlying)
	var staged []*teamSyncResponseWriter
	for i := 0; i < cap(teamSyncResponseSlots); i++ {
		writer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
		require.NoError(t, err)
		staged = append(staged, writer)
	}
	t.Cleanup(func() {
		for _, writer := range staged {
			_ = writer.close() // each created file is closed individually
		}
	})
	_, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.ErrorContains(t, err, "busy")
	assert.Empty(t, underlying.Body.String())
	require.NoError(t, staged[0].close())
	replacement, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	require.NoError(t, replacement.close())
}

func TestTeamSyncResponseRefusesOversizeAndDiskWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail func(*teamSyncResponseWriter)
	}{
		{"size limit", func(w *teamSyncResponseWriter) { w.size = maxTeamSyncResponseBytes }},
		{"closed staging file", func(w *teamSyncResponseWriter) { _ = w.file.Close() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			underlying := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(underlying)
			writer, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writer.close() })
			tc.fail(writer)
			_, err = writer.Write([]byte("private upstream result"))
			require.Error(t, err)
			require.Error(t, writer.commit())
			assert.Empty(t, underlying.Body.String())
		})
	}
}

func TestTeamSyncResponseSharedBudgetAndDiskHeadroom(t *testing.T) {
	underlying := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(underlying)
	first, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	defer first.close()
	second, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	defer second.close()
	baseline := teamStagedBytes.Load()
	teamStagedBytes.Store(maxTeamStagedBytes - 4)
	defer teamStagedBytes.Store(baseline)
	_, err = first.WriteString("12345")
	require.ErrorContains(t, err, "capacity")
	assert.Equal(t, maxTeamStagedBytes-4, teamStagedBytes.Load())
	_, err = second.WriteString("1234")
	require.NoError(t, err)
	assert.Equal(t, maxTeamStagedBytes, teamStagedBytes.Load())
	require.NoError(t, second.close())
	assert.Equal(t, maxTeamStagedBytes-4, teamStagedBytes.Load())
	assert.Empty(t, underlying.Body.String())
	teamStagedBytes.Store(baseline)

	oldAvailable := teamStagingAvailable
	teamStagingAvailable = func(string) (int64, error) { return minTeamStagingFreeBytes, nil }
	defer func() { teamStagingAvailable = oldAvailable }()
	third, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	defer third.close()
	_, err = third.WriteString("charged result")
	require.ErrorContains(t, err, "insufficient disk space")
	assert.Equal(t, baseline, teamStagedBytes.Load())
	require.Error(t, third.commit())
	assert.Empty(t, underlying.Body.String())
}

func TestTeamSyncResponseConcurrentWritersPreserveSharedDiskHeadroom(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	first, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	defer first.close()
	second, err := newTeamSyncResponseWriter(stdctx.Background(), context.Writer)
	require.NoError(t, err)
	defer second.close()
	previousAvailable := teamStagingAvailable
	teamStagingAvailable = func(string) (int64, error) {
		firstInfo, err := first.file.Stat()
		if err != nil {
			return 0, err
		}
		secondInfo, err := second.file.Stat()
		if err != nil {
			return 0, err
		}
		return minTeamStagingFreeBytes + 1 - firstInfo.Size() - secondInfo.Size(), nil
	}
	defer func() { teamStagingAvailable = previousAvailable }()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, writer := range []*teamSyncResponseWriter{first, second} {
		go func(w *teamSyncResponseWriter) {
			<-start
			_, err := w.WriteString("x")
			results <- err
		}(writer)
	}
	close(start)
	var succeeded, rejected int
	for i := 0; i < 2; i++ {
		if err := <-results; err == nil {
			succeeded++
		} else {
			require.ErrorContains(t, err, "insufficient disk space")
			rejected++
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, rejected)
	assert.Empty(t, recorder.Body.String())
}

func TestTeamSyncResponseCanceledBeforeDeliveryStaysPrivate(t *testing.T) {
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	underlying := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(underlying)
	writer, err := newTeamSyncResponseWriter(ctx, ginContext.Writer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.close() })
	writer.Header().Set("X-Team-Result", "private")
	_, err = writer.WriteString("charged upstream result")
	require.NoError(t, err)
	cancel()
	require.ErrorIs(t, writer.commit(), stdctx.Canceled)
	assert.Empty(t, underlying.Body.String())
	assert.Empty(t, underlying.Header().Get("X-Team-Result"))
}

func TestTeamSyncResponseStopsStagingAfterClientDisconnect(t *testing.T) {
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	defer cancel()
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	writer, err := newTeamSyncResponseWriter(ctx, ginContext.Writer)
	require.NoError(t, err)
	defer writer.close()
	cancel()
	_, err = writer.WriteString("private upstream response")
	require.ErrorIs(t, err, stdctx.Canceled)
	assert.Zero(t, writer.size)
	assert.Zero(t, writer.reserved)
	assert.Empty(t, recorder.Body.String())
}

func TestTeamSyncResponseStopsCopyAfterClientDisconnect(t *testing.T) {
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	defer cancel()
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	client := &cancelingTeamResponseWriter{ResponseWriter: ginContext.Writer, cancel: cancel}
	writer, err := newTeamSyncResponseWriter(ctx, client)
	require.NoError(t, err)
	defer writer.close()
	_, err = writer.WriteString(strings.Repeat("s", 128<<10))
	require.NoError(t, err)
	err = writer.commit()
	require.True(t, errors.Is(err, stdctx.Canceled))
	assert.Equal(t, 1, client.writes)
	assert.LessOrEqual(t, recorder.Body.Len(), 32<<10)
}

func TestTeamSyncResponseCannotBypassStagingViaHijackOrPush(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	underlying := &trackingTeamHijackWriter{ResponseWriter: context.Writer}
	writer, err := newTeamSyncResponseWriter(stdctx.Background(), underlying)
	require.NoError(t, err)
	defer writer.close()
	writer.Header().Set("X-Private-Team-Result", "not-settled")
	_, err = writer.WriteString("private result")
	require.NoError(t, err)
	connection, buffered, err := writer.Hijack()
	require.ErrorContains(t, err, "cannot hijack")
	assert.Nil(t, connection)
	assert.Nil(t, buffered)
	assert.Zero(t, underlying.hijacks)
	assert.Nil(t, writer.Pusher())
	require.Error(t, writer.commit())
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("X-Private-Team-Result"))
}
