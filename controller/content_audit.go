package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func UpdateContentAuditSetting(c *gin.Context) {
	var request struct {
		Enabled      bool `json:"enabled"`
		PrivacyReady bool `json:"privacy_ready"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的内容审计配置")
		return
	}
	if request.Enabled && !request.PrivacyReady {
		common.ApiErrorMsg(c, "启用内容审计前必须确认隐私政策已就绪")
		return
	}
	if request.Enabled && !model.ContentAuditEncryptionReady() {
		common.ApiErrorMsg(c, "服务器未配置有效的内容审计加密密钥")
		return
	}
	values := map[string]string{
		"content_audit.enabled":       strconv.FormatBool(request.Enabled),
		"content_audit.privacy_ready": strconv.FormatBool(request.PrivacyReady),
	}
	if err := model.UpdateOptionsBulkWithActivationKey(values, "content_audit.enabled"); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "content_audit.setting.update", map[string]interface{}{"enabled": request.Enabled, "privacy_ready": request.PrivacyReady})
	common.ApiSuccess(c, gin.H{"enabled": request.Enabled, "privacy_ready": request.PrivacyReady, "retention_days": 7, "encryption_ready": model.ContentAuditEncryptionReady()})
}

func ListContentAudits(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Query("user_id"))
	startAt, _ := strconv.ParseInt(c.Query("start_at"), 10, 64)
	endAt, _ := strconv.ParseInt(c.Query("end_at"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	rows, total, err := model.ListContentAudits(userID, c.Query("model"), c.Query("request_id"), c.Query("source"), c.Query("status"), startAt, endAt, pageSize, (page-1)*pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": rows, "total": total, "page": page, "page_size": pageSize})
}

func GetContentAudit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的审计记录编号")
		return
	}
	detail, err := model.GetContentAuditDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "content_audit.detail.view", map[string]interface{}{"audit_id": id, "request_id": detail.RequestId, "target_user_id": detail.UserId})
	common.ApiSuccess(c, detail)
}
