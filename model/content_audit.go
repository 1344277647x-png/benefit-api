package model

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const contentAuditFieldLimit = 65536

var auditSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|sk-|api[_-]?key["'\s:=]+|token["'\s:=]+|cookie["'\s:=]+|card[_-]?(?:number|no)["'\s:=]+)[^\s,"'}]{6,}`)
var auditSecretKeyPattern = regexp.MustCompile(`(?i)(authorization|cookie|api[_-]?key|access[_-]?token|refresh[_-]?token|password|secret|card[_-]?(?:number|no))`)
var auditHiddenKeyPattern = regexp.MustCompile(`(?i)^(system|system_prompt|systemInstruction|system_instruction|instructions|developer|reasoning|reasoning_content|thinking|thought|thoughts)$`)
var auditBinaryKeyPattern = regexp.MustCompile(`(?i)^(b64_json|image_data|audio_data|video_data|file_data|binary|bytes)$`)
var auditDataURIPattern = regexp.MustCompile(`(?i)data:[^;,\s]{1,128};base64,[a-z0-9+/=_-]+`)

type ContentAuditRecord struct {
	Id               int64  `json:"id"`
	UserId           int    `json:"user_id" gorm:"index"`
	TeamId           int    `json:"team_id" gorm:"index"`
	ModelName        string `json:"model_name" gorm:"type:varchar(191);index"`
	RequestId        string `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	Source           string `json:"source" gorm:"type:varchar(32);index"`
	Status           string `json:"status" gorm:"type:varchar(32);index"`
	InputCiphertext  string `json:"-" gorm:"type:text"`
	OutputCiphertext string `json:"-" gorm:"type:text"`
	ResultReferences string `json:"-" gorm:"type:text"`
	InputTruncated   bool   `json:"input_truncated"`
	OutputTruncated  bool   `json:"output_truncated"`
	CreatedAt        int64  `json:"created_at" gorm:"type:bigint;index"`
	ExpiresAt        int64  `json:"expires_at" gorm:"type:bigint;index"`
}

type ContentAuditDetail struct {
	ContentAuditRecord
	Input            string `json:"input"`
	Output           string `json:"output"`
	ResultReferences string `json:"result_references"`
}

func contentAuditAEAD() (cipher.AEAD, error) {
	raw := strings.TrimSpace(os.Getenv("CONTENT_AUDIT_ENCRYPTION_KEY"))
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("CONTENT_AUDIT_ENCRYPTION_KEY must be a base64 encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func ContentAuditEncryptionReady() bool { _, err := contentAuditAEAD(); return err == nil }

func encryptAuditText(value string) (string, bool, error) {
	oversized := len(value) > 4*contentAuditFieldLimit
	if oversized {
		// Fail closed instead of parsing or slicing incomplete credential-bearing JSON.
		value = "[AUDIT_CONTENT_OMITTED_SIZE_LIMIT]"
	}
	value = sanitizeAuditText(value)
	truncated := oversized || len(value) > contentAuditFieldLimit
	if len(value) > contentAuditFieldLimit {
		value = value[:contentAuditFieldLimit]
		for !utf8.ValidString(value) && len(value) > 0 {
			value = value[:len(value)-1]
		}
	}
	aead, err := contentAuditAEAD()
	if err != nil {
		return "", truncated, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return "", truncated, err
	}
	sealed := aead.Seal(nonce, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(sealed), truncated, nil
}

func sanitizeAuditText(value string) string {
	var decoded any
	if common.UnmarshalJsonStr(value, &decoded) == nil {
		var scrub func(any) any
		scrub = func(current any) any {
			switch typed := current.(type) {
			case map[string]any:
				if hidden, _ := typed["thought"].(bool); hidden {
					return "[REDACTED]"
				}
				for key, item := range typed {
					if auditSecretKeyPattern.MatchString(key) || auditHiddenKeyPattern.MatchString(key) || auditBinaryKeyPattern.MatchString(key) {
						typed[key] = "[REDACTED]"
					} else if key == "messages" {
						if messages, ok := item.([]any); ok {
							filtered := make([]any, 0, len(messages))
							for _, message := range messages {
								entry, ok := message.(map[string]any)
								role, _ := entry["role"].(string)
								role = strings.ToLower(role)
								if ok && (role == "system" || role == "developer") {
									continue
								}
								filtered = append(filtered, scrub(message))
							}
							typed[key] = filtered
						} else {
							typed[key] = scrub(item)
						}
					} else {
						typed[key] = scrub(item)
					}
				}
				return typed
			case []any:
				filtered := make([]any, 0, len(typed))
				for _, item := range typed {
					if entry, ok := item.(map[string]any); ok {
						role, _ := entry["role"].(string)
						role = strings.ToLower(role)
						if role == "system" || role == "developer" {
							continue
						}
					}
					filtered = append(filtered, scrub(item))
				}
				return filtered
			case string:
				return auditSecretPattern.ReplaceAllString(auditDataURIPattern.ReplaceAllString(typed, "[MEDIA_DATA_REDACTED]"), "[REDACTED]")
			default:
				return current
			}
		}
		if encoded, err := common.Marshal(scrub(decoded)); err == nil {
			value = string(encoded)
		}
	}
	if strings.Contains(value, "data:") {
		lines := strings.Split(value, "\n")
		kept := lines[:0]
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "reasoning") || strings.Contains(lower, "thinking") {
				continue
			}
			kept = append(kept, line)
		}
		value = strings.Join(kept, "\n")
	}
	value = auditDataURIPattern.ReplaceAllString(value, "[MEDIA_DATA_REDACTED]")
	return auditSecretPattern.ReplaceAllString(value, "[REDACTED]")
}

func finalVisibleAuditOutput(value string) string {
	if len(value) > 4*contentAuditFieldLimit {
		return "[AUDIT_CONTENT_OMITTED_SIZE_LIMIT]"
	}
	lines := strings.Split(value, "\n")
	var visible strings.Builder
	sawSSE := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		sawSSE = true
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event map[string]any
		if common.UnmarshalJsonStr(payload, &event) != nil {
			continue
		}
		visible.WriteString(visibleTextFromAuditEvent(event))
	}
	if sawSSE {
		return visible.String()
	}
	return value
}

func visibleTextFromAuditEvent(event map[string]any) string {
	eventType, _ := event["type"].(string)
	lowerType := strings.ToLower(eventType)
	if strings.Contains(lowerType, "reasoning") || strings.Contains(lowerType, "thinking") || strings.HasSuffix(lowerType, ".completed") {
		return ""
	}
	if lowerType == "response.output_text.delta" {
		text, _ := event["delta"].(string)
		return text
	}
	if delta, ok := event["delta"].(map[string]any); ok {
		deltaType, _ := delta["type"].(string)
		if !strings.Contains(strings.ToLower(deltaType), "thinking") && !strings.Contains(strings.ToLower(deltaType), "reasoning") {
			if text, ok := delta["text"].(string); ok {
				return text
			}
			if content, ok := delta["content"].(string); ok {
				return content
			}
		}
	}
	if choices, ok := event["choices"].([]any); ok {
		var text strings.Builder
		for _, rawChoice := range choices {
			choice, _ := rawChoice.(map[string]any)
			for _, field := range []string{"delta", "message"} {
				container, _ := choice[field].(map[string]any)
				if content, ok := container["content"].(string); ok {
					text.WriteString(content)
				}
			}
			if content, ok := choice["text"].(string); ok {
				text.WriteString(content)
			}
		}
		return text.String()
	}
	if candidates, ok := event["candidates"].([]any); ok {
		var text strings.Builder
		for _, rawCandidate := range candidates {
			candidate, _ := rawCandidate.(map[string]any)
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if hidden, _ := part["thought"].(bool); hidden {
					continue
				}
				if value, ok := part["text"].(string); ok {
					text.WriteString(value)
				}
			}
		}
		return text.String()
	}
	if auditError, ok := event["error"].(map[string]any); ok {
		message, _ := auditError["message"].(string)
		return message
	}
	return ""
}

func decryptAuditText(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	aead, err := contentAuditAEAD()
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(sealed) < aead.NonceSize() {
		return "", errors.New("invalid audit ciphertext")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
	return string(plain), err
}

func InsertContentAudit(record *ContentAuditRecord, input, output, resultReferences string) error {
	if record == nil || record.UserId <= 0 || record.RequestId == "" {
		return errors.New("invalid content audit record")
	}
	var err error
	record.InputCiphertext, record.InputTruncated, err = encryptAuditText(input)
	if err != nil {
		return err
	}
	record.OutputCiphertext, record.OutputTruncated, err = encryptAuditText(finalVisibleAuditOutput(output))
	record.OutputTruncated = record.OutputTruncated || len(output) > 4*contentAuditFieldLimit
	if err != nil {
		return err
	}
	record.ResultReferences, _, err = encryptAuditText(resultReferences)
	if err != nil {
		return err
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = common.GetTimestamp()
	}
	record.ExpiresAt = record.CreatedAt + 7*24*60*60
	return DB.Create(record).Error
}

func ListContentAudits(userID int, modelName, requestID, source, status string, startAt, endAt int64, limit, offset int) ([]ContentAuditRecord, int64, error) {
	query := DB.Model(&ContentAuditRecord{}).Where("expires_at > ?", common.GetTimestamp())
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	if requestID != "" {
		query = query.Where("request_id = ?", requestID)
	}
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if startAt > 0 {
		query = query.Where("created_at >= ?", startAt)
	}
	if endAt > 0 {
		query = query.Where("created_at <= ?", endAt)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ContentAuditRecord
	err := query.Select("id", "user_id", "team_id", "model_name", "request_id", "source", "status", "input_truncated", "output_truncated", "created_at", "expires_at").Order("id desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func GetContentAuditDetail(id int64) (*ContentAuditDetail, error) {
	var record ContentAuditRecord
	if err := DB.Where("expires_at > ?", common.GetTimestamp()).First(&record, id).Error; err != nil {
		return nil, err
	}
	input, err := decryptAuditText(record.InputCiphertext)
	if err != nil {
		return nil, err
	}
	output, err := decryptAuditText(record.OutputCiphertext)
	if err != nil {
		return nil, err
	}
	refs, err := decryptAuditText(record.ResultReferences)
	if err != nil {
		return nil, err
	}
	return &ContentAuditDetail{ContentAuditRecord: record, Input: input, Output: output, ResultReferences: refs}, nil
}

func DeleteExpiredContentAudits(now int64) (int64, error) {
	var ids []int64
	if err := DB.Model(&ContentAuditRecord{}).Where("expires_at > 0 AND expires_at <= ?", now).Order("id asc").Limit(1000).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Where("id IN ? AND expires_at > 0 AND expires_at <= ?", ids, now).Delete(&ContentAuditRecord{})
	return result.RowsAffected, result.Error
}
