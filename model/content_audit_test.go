package model

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentAuditEncryptsAndRedactsCredentials(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	record := &ContentAuditRecord{UserId: 42, RequestId: "audit-request-1", ModelName: "test-model", Source: "api", Status: "succeeded"}
	require.NoError(t, InsertContentAudit(record, `{"prompt":"hello","Authorization":"Bearer secret-token-value","image_url":"data:image/png;base64,QUJDREVGRw==","b64_json":"SElEREVO","input":[{"role":"system","content":"hidden prompt"},{"role":"user","content":"visible prompt"}]}`, `{"answer":"ok","api_key":"secret-value"}`, "asset-1"))
	assert.NotContains(t, record.InputCiphertext, "hello")
	assert.NotContains(t, record.OutputCiphertext, "secret-value")
	detail, err := GetContentAuditDetail(record.Id)
	require.NoError(t, err)
	assert.Contains(t, detail.Input, "[REDACTED]")
	assert.Contains(t, detail.Input, "[MEDIA_DATA_REDACTED]")
	assert.NotContains(t, detail.Input, "secret-token-value")
	assert.NotContains(t, detail.Input, "QUJDREVGRw==")
	assert.NotContains(t, detail.Input, "SElEREVO")
	assert.NotContains(t, detail.Input, "hidden prompt")
	assert.Contains(t, detail.Input, "visible prompt")
	assert.NotContains(t, detail.Output, "secret-value")
	assert.Equal(t, "asset-1", detail.ResultReferences)
	t.Cleanup(func() { _ = DB.Delete(record).Error })
}

func TestContentAuditTruncatesBeforeEncryption(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	record := &ContentAuditRecord{UserId: 43, RequestId: "audit-request-2", Source: "api", Status: "succeeded"}
	require.NoError(t, InsertContentAudit(record, strings.Repeat("x", contentAuditFieldLimit+10), "ok", ""))
	assert.True(t, record.InputTruncated)
	detail, err := GetContentAuditDetail(record.Id)
	require.NoError(t, err)
	assert.Len(t, detail.Input, contentAuditFieldLimit)
	t.Cleanup(func() { _ = DB.Delete(record).Error })
}

func TestContentAuditStoresOnlyVisibleStreamingOutput(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	record := &ContentAuditRecord{UserId: 44, RequestId: "audit-request-stream", Source: "api", Status: "succeeded"}
	stream := "data: {\"type\":\"response.reasoning.delta\",\"delta\":\"hidden\"}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n" +
		"data: [DONE]\n"
	require.NoError(t, InsertContentAudit(record, "prompt", stream, ""))
	detail, err := GetContentAuditDetail(record.Id)
	require.NoError(t, err)
	assert.Equal(t, "Hello world", detail.Output)
	assert.NotContains(t, detail.Output, "hidden")
	t.Cleanup(func() { _ = DB.Delete(record).Error })
}
