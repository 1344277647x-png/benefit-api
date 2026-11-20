package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeQixiangConfirmation struct {
	money string
	err   error
	calls int
}

func (f *fakeQixiangConfirmation) ConfirmWeChatPayment(context.Context, service.QixiangPaymentNotice) (string, error) {
	f.calls++
	return f.money, f.err
}

func TestQixiangCreditsOnlyAfterPaymentConfirmation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	oldDB, oldLogDB, oldRedis, oldQuota := model.DB, model.LOG_DB, common.RedisEnabled, common.QuotaPerUnit
	model.DB, model.LOG_DB, common.RedisEnabled, common.QuotaPerUnit = db, db, false, 500000
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.QuotaPerUnit = oldDB, oldLogDB, oldRedis, oldQuota
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.TopUp{}, &model.BusinessEvent{}))
	user := model.User{Username: "test-qixiang", Quota: 100, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	order := model.TopUp{TradeNo: "QXtest", UserId: user.Id, Amount: 1, Money: 7.30,
		Status: common.TopUpStatusPending, PaymentMethod: model.PaymentMethodQixiangWeChat, PaymentProvider: model.PaymentProviderQixiang}
	require.NoError(t, db.Create(&order).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/qixiang/notify", nil)
	notice := service.QixiangPaymentNotice{TradeNo: order.TradeNo, Money: "7.30"}
	confirmer := &fakeQixiangConfirmation{err: errors.New("still unpaid")}
	require.Error(t, model.ManualCompleteTopUp(order.TradeNo, ""))
	require.Error(t, settleQixiangNotice(ctx, confirmer, notice))
	confirmer.err, confirmer.money = nil, "7.31"
	require.Error(t, settleQixiangNotice(ctx, confirmer, notice))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	confirmer.money = "7.30"
	require.NoError(t, settleQixiangNotice(ctx, confirmer, notice))
	require.NoError(t, settleQixiangNotice(ctx, confirmer, notice))
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 500100, user.Quota)
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, 3, confirmer.calls)
}
