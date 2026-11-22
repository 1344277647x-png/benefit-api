package controller

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Task adapters write their success response before the task is persisted.
// Keep team responses private until the task and its reservation are durable.
const maxTeamTaskResponseBytes = 2 << 20

type teamTaskResponseWriter struct {
	gin.ResponseWriter
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
	err     error
}

func newTeamTaskResponseWriter(original gin.ResponseWriter) *teamTaskResponseWriter {
	return &teamTaskResponseWriter{ResponseWriter: original, header: original.Header().Clone(), status: http.StatusOK}
}

func (w *teamTaskResponseWriter) Header() http.Header  { return w.header }
func (w *teamTaskResponseWriter) WriteHeader(code int) { w.status = code }
func (w *teamTaskResponseWriter) WriteHeaderNow()      { w.written = true }
func (w *teamTaskResponseWriter) Status() int          { return w.status }
func (w *teamTaskResponseWriter) Size() int            { return w.body.Len() }
func (w *teamTaskResponseWriter) Written() bool        { return w.written }
func (w *teamTaskResponseWriter) Write(b []byte) (int, error) {
	w.written = true
	if len(b) > maxTeamTaskResponseBytes-w.body.Len() {
		w.err = errors.New("team task response exceeds size limit")
		return 0, w.err
	}
	return w.body.Write(b)
}
func (w *teamTaskResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *teamTaskResponseWriter) Flush() {
	// Task submit adapters are JSON-only. Never let a future adapter flush an
	// unpersisted task ID through the embedded writer.
	w.err = errors.New("streaming team task submission is unsupported")
}

func (w *teamTaskResponseWriter) commit() error {
	if w.err != nil {
		return w.err
	}
	for key := range w.ResponseWriter.Header() {
		w.ResponseWriter.Header().Del(key)
	}
	for key, values := range w.header {
		w.ResponseWriter.Header()[key] = append([]string(nil), values...)
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	return err
}
