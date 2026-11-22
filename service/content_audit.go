package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type ContentAuditWrite struct {
	UserID           int
	TeamID           int
	ModelName        string
	RequestID        string
	Source           string
	Status           string
	Input            string
	Output           string
	ResultReferences string
}

// RecordContentAudit is best-effort: audit storage must not change a paid AI
// response, while failures remain visible in the server's administrator log.
func RecordContentAudit(write ContentAuditWrite) {
	setting := operation_setting.GetContentAuditSettingSnapshot()
	if !setting.Enabled || !setting.PrivacyReady {
		return
	}
	record := &model.ContentAuditRecord{UserId: write.UserID, TeamId: write.TeamID,
		ModelName: write.ModelName, RequestId: write.RequestID, Source: write.Source, Status: write.Status}
	if err := model.InsertContentAudit(record, write.Input, write.Output, write.ResultReferences); err != nil {
		common.SysError("content audit write failed for request " + common.LocalLogPreview(write.RequestID) + ": " + err.Error())
	}
}
