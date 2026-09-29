package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetLotteryStatus(c *gin.Context) {
	status, err := model.GetLotteryStatus(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, status)
}

func DrawLottery(c *gin.Context) {
	requestKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	draw, status, err := model.DrawLottery(c.GetInt("id"), requestKey)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrLotteryRequestKeyRequired):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Idempotency-Key is required and must not exceed 128 characters"})
		case errors.Is(err, model.ErrLotteryInactive):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "The lottery activity is not active"})
		case errors.Is(err, model.ErrLotteryNoDraws):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "No lottery draws are available"})
		case errors.Is(err, model.ErrLotteryRequestConflict):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Lottery state changed; retry with a new Idempotency-Key"})
		default:
			common.ApiError(c, err)
		}
		return
	}
	_ = service.DeliverPendingBusinessEvents(c.Request.Context(), 100)
	common.ApiSuccess(c, gin.H{"draw": draw, "status": status})
}

func GetLotteryHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	history, err := model.GetLotteryHistory(c.GetInt("id"), page, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, history)
}
