package controller

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

type contentAuditResponseWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	status    int
	truncated bool
}

func newContentAuditResponseWriter(writer gin.ResponseWriter) *contentAuditResponseWriter {
	return &contentAuditResponseWriter{ResponseWriter: writer, status: http.StatusOK}
}

func (writer *contentAuditResponseWriter) WriteHeader(code int) {
	writer.status = code
	writer.ResponseWriter.WriteHeader(code)
}

func (writer *contentAuditResponseWriter) Write(data []byte) (int, error) {
	remaining := contentAuditResponseLimit - writer.body.Len()
	if remaining > 0 {
		copyLength := min(remaining, len(data))
		_, _ = writer.body.Write(data[:copyLength])
	}
	if len(data) > remaining {
		writer.truncated = true
	}
	return writer.ResponseWriter.Write(data)
}

func (writer *contentAuditResponseWriter) WriteString(data string) (int, error) {
	remaining := contentAuditResponseLimit - writer.body.Len()
	if remaining > 0 {
		copyLength := min(remaining, len(data))
		_, _ = writer.body.WriteString(data[:copyLength])
	}
	if len(data) > remaining {
		writer.truncated = true
	}
	return writer.ResponseWriter.WriteString(data)
}

const contentAuditResponseLimit = 65536
