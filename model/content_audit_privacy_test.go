package model

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentAuditHidesGeminiThoughtAndExpiredContent(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	input := `{"systemInstruction":{"parts":[{"text":"hidden-system"}]},"prompt":"visible"}`
	output := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"thought\":true,\"text\":\"hidden-thought\"},{\"text\":\"answer\"}]}}]}\n"
	record := &ContentAuditRecord{UserId: 42, RequestId: t.Name()}
	require.NoError(t, InsertContentAudit(record, input, output, "asset"))
	detail, err := GetContentAuditDetail(record.Id)
	require.NoError(t, err)
	assert.NotContains(t, detail.Input, "hidden-system")
	assert.NotContains(t, detail.Output, "hidden-thought")
	assert.Contains(t, detail.Output, "answer")
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(record).Update("expires_at", now-1).Error)
	_, err = GetContentAuditDetail(record.Id)
	require.Error(t, err)
	_, err = DeleteExpiredContentAudits(now)
	require.NoError(t, err)
	assert.Error(t, DB.First(&ContentAuditRecord{}, record.Id).Error)
}

func TestAuditTruncationKeepsUTF8AndOmitsOversizedJSON(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	sealed, truncated, err := encryptAuditText(strings.Repeat("中", contentAuditFieldLimit/3+2))
	require.NoError(t, err)
	assert.True(t, truncated)
	plain, err := decryptAuditText(sealed)
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(plain))
	sealed, truncated, err = encryptAuditText(`{"prompt":"` + strings.Repeat("x", 4*contentAuditFieldLimit) + `","password":"sensitive"}`)
	require.NoError(t, err)
	assert.True(t, truncated)
	plain, err = decryptAuditText(sealed)
	require.NoError(t, err)
	assert.Equal(t, "[AUDIT_CONTENT_OMITTED_SIZE_LIMIT]", plain)
}
