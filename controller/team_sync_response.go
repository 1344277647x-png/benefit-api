package controller

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// A synchronous team response cannot become visible before its charge has
// committed. Live team streams are rejected by Relay rather than staged.
const (
	maxTeamSyncResponseBytes int64 = 320 << 20
	maxTeamStagedBytes       int64 = 512 << 20
	minTeamStagingFreeBytes  int64 = 64 << 20
)

// Bound disk pressure from concurrent, uncommitted team responses.
var teamSyncResponseSlots = make(chan struct{}, 4)
var teamStagedBytes atomic.Int64
var teamStagingAvailable = teamStagingFreeBytes

// Checking free space and writing must be serialized across responses. If
// each of four writers checks before any one writes, all can independently
// pass the same disk-headroom check and exhaust the volume together.
var teamStagingDiskMu sync.Mutex

func reserveTeamStagingBytes(amount int64) bool {
	for {
		current := teamStagedBytes.Load()
		if amount > maxTeamStagedBytes-current {
			return false
		}
		if teamStagedBytes.CompareAndSwap(current, current+amount) {
			return true
		}
	}
}

func teamSyncStagingDir() (string, error) {
	if runtime.GOOS != "windows" {
		return "", nil // Linux unlinks each open temporary file immediately.
	}
	dir := os.Getenv("TEAM_RESPONSE_STAGING_DIR")
	if dir == "" {
		dir = `D:\BenefitAPI\team-response-staging`
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.VolumeName(dir), "D:") {
		return "", errors.New("team response staging must be on the D drive")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

type teamSyncResponseWriter struct {
	gin.ResponseWriter
	ctx      context.Context
	header   http.Header
	file     *os.File
	status   int
	size     int64
	reserved int64
	written  bool
	err      error
	mu       sync.Mutex
}

func newTeamSyncResponseWriter(ctx context.Context, original gin.ResponseWriter) (*teamSyncResponseWriter, error) {
	select {
	case teamSyncResponseSlots <- struct{}{}:
	default:
		return nil, errors.New("team response staging is busy")
	}
	dir, err := teamSyncStagingDir()
	if err != nil {
		<-teamSyncResponseSlots
		return nil, err
	}
	file, err := createTeamSyncStagingFile(dir)
	if err != nil {
		<-teamSyncResponseSlots
		return nil, err
	}
	if runtime.GOOS != "windows" {
		// On Linux the open descriptor remains usable; a process crash cannot
		// leave an orphaned response file with potentially sensitive output.
		if err := os.Remove(file.Name()); err != nil {
			_ = file.Close()
			<-teamSyncResponseSlots
			return nil, err
		}
	}
	return &teamSyncResponseWriter{ResponseWriter: original, ctx: ctx, header: original.Header().Clone(), file: file, status: http.StatusOK}, nil
}

func (w *teamSyncResponseWriter) Header() http.Header  { return w.header }
func (w *teamSyncResponseWriter) WriteHeader(code int) { w.status = code }
func (w *teamSyncResponseWriter) WriteHeaderNow()      { w.written = true }
func (w *teamSyncResponseWriter) Status() int          { return w.status }
func (w *teamSyncResponseWriter) Size() int            { return int(w.size) }
func (w *teamSyncResponseWriter) Written() bool        { return w.written }
func (w *teamSyncResponseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.written = true
	if w.err != nil {
		return 0, w.err
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return 0, err
	}
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if int64(len(b)) > maxTeamSyncResponseBytes-w.size {
		w.err = errors.New("team response exceeds buffer limit")
		return 0, w.err
	}
	if !reserveTeamStagingBytes(int64(len(b))) {
		w.err = errors.New("team response staging capacity exceeded")
		return 0, w.err
	}
	teamStagingDiskMu.Lock()
	defer teamStagingDiskMu.Unlock()
	free, err := teamStagingAvailable(w.file.Name())
	if err != nil || free < minTeamStagingFreeBytes+int64(len(b)) {
		teamStagedBytes.Add(-int64(len(b)))
		w.err = errors.New("team response staging has insufficient disk space")
		return 0, w.err
	}
	n, err := w.file.Write(b)
	w.size += int64(n)
	w.reserved += int64(n)
	teamStagedBytes.Add(int64(n - len(b)))
	w.err = err
	return n, err
}
func (w *teamSyncResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *teamSyncResponseWriter) Flush() {
	// A streaming adapter may call Flush, but no uncharged bytes may escape.
}

func (w *teamSyncResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Gin's ResponseWriter embeds http.Hijacker. Delegating it would hand an
	// adapter the raw client connection and bypass the settlement boundary.
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = errors.New("team response cannot hijack the client connection")
	return nil, nil, w.err
}

func (w *teamSyncResponseWriter) Pusher() http.Pusher {
	// A pushed response would also bypass the staged body and its charge.
	return nil
}

func (w *teamSyncResponseWriter) commit() error {
	if w.err != nil {
		return w.err
	}
	// The upstream may finish after the client disconnects. Keep a charged
	// result private if there is no longer a recipient; the request status
	// endpoint can report the resulting uncertain delivery separately.
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for key := range w.ResponseWriter.Header() {
		w.ResponseWriter.Header().Del(key)
	}
	for key, values := range w.header {
		w.ResponseWriter.Header()[key] = append([]string(nil), values...)
	}
	w.ResponseWriter.WriteHeader(w.status)
	buffer := make([]byte, 32<<10)
	for {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		n, readErr := w.file.Read(buffer)
		if n > 0 {
			written, writeErr := w.ResponseWriter.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (w *teamSyncResponseWriter) close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	path := w.file.Name()
	closeErr := w.file.Close()
	w.file = nil
	teamStagedBytes.Add(-w.reserved)
	w.reserved = 0
	<-teamSyncResponseSlots
	removeErr := os.Remove(path) // one generated file, never a directory
	if closeErr != nil {
		return closeErr
	}
	if os.IsNotExist(removeErr) {
		return nil
	}
	return removeErr
}
