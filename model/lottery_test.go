package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureLotteryTest(t *testing.T, enabled bool) {
	t.Helper()
	setting := operation_setting.GetLotterySetting()
	originalSetting := *setting
	originalQuotaPerUnit := common.QuotaPerUnit
	originalPrice := operation_setting.Price
	*setting = operation_setting.LotterySetting{Enabled: enabled}
	common.QuotaPerUnit = 100
	operation_setting.Price = 2
	t.Cleanup(func() {
		*setting = originalSetting
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.Price = originalPrice
	})
}

func insertLotteryUser(t *testing.T, userId int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{Id: userId, Username: "lottery-user", Status: common.UserStatusEnabled}).Error)
}

func insertLotteryTopUp(t *testing.T, userId int, tradeNo string, money float64, provider string, method string) {
	t.Helper()
	topUp := TopUp{
		UserId: userId, Amount: int64(money), Money: money, TradeNo: tradeNo,
		PaymentMethod: method, PaymentProvider: provider,
		Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
	}
	require.NoError(t, topUp.Insert())
}

func TestLotteryRewardPoolBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		tier        string
		roll        int64
		rewardCents int64
	}{
		{name: "regular last half yuan", tier: "regular", roll: 74998, rewardCents: 50},
		{name: "regular first one yuan", tier: "regular", roll: 74999, rewardCents: 100},
		{name: "regular first two yuan", tier: "regular", roll: 94999, rewardCents: 200},
		{name: "regular hundred yuan", tier: "regular", roll: 99999, rewardCents: 10000},
		{name: "jackpot last five yuan", tier: "jackpot", roll: 79998, rewardCents: 500},
		{name: "jackpot first ten yuan", tier: "jackpot", roll: 79999, rewardCents: 1000},
		{name: "jackpot first fifty yuan", tier: "jackpot", roll: 94999, rewardCents: 5000},
		{name: "jackpot hundred yuan", tier: "jackpot", roll: 99999, rewardCents: 10000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reward, err := lotteryRewardForRoll(test.tier, test.roll)
			require.NoError(t, err)
			assert.Equal(t, test.rewardCents, reward)
		})
	}
}

func TestLotteryRewardQuotaUsesCnyPriceWithoutTopupMultiplier(t *testing.T) {
	configureLotteryTest(t, true)
	quota, err := lotteryRewardQuota(100)
	require.NoError(t, err)
	assert.Equal(t, 50, quota)
}

func TestLotteryGrantAccumulatesOnlySuccessfulWalletTopUps(t *testing.T) {
	truncateTables(t)
	configureLotteryTest(t, true)
	insertLotteryUser(t, 1201)
	insertLotteryTopUp(t, 1201, "lottery-20", 20, PaymentProviderAlipay, PaymentMethodAlipayNative)
	insertLotteryTopUp(t, 1201, "lottery-30", 30, PaymentProviderEpay, "alipay")

	require.NoError(t, CompleteAlipayTopUp("lottery-20", "20.00", "127.0.0.1"))
	_, err := RechargeEpay("lottery-30", "alipay", "127.0.0.1")
	require.NoError(t, err)

	status, err := GetLotteryStatus(1201)
	require.NoError(t, err)
	assert.Equal(t, 1, status.AvailableDraws)
	assert.Zero(t, status.RechargeRemainderCents)
	assert.Equal(t, int64(5000), status.TotalEligibleRechargeCents)

	alreadyDone, err := RechargeEpay("lottery-30", "alipay", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	status, err = GetLotteryStatus(1201)
	require.NoError(t, err)
	assert.Equal(t, 1, status.AvailableDraws)
	assert.Equal(t, int64(5000), status.TotalEligibleRechargeCents)

	var grants int64
	require.NoError(t, DB.Model(&LotteryGrant{}).Count(&grants).Error)
	assert.Equal(t, int64(2), grants)
}

func TestLotteryDrawIsIdempotentAndCreditsQuotaOnce(t *testing.T) {
	truncateTables(t)
	configureLotteryTest(t, true)
	insertLotteryUser(t, 1202)
	require.NoError(t, DB.Create(&LotteryAccount{UserId: 1202, AvailableDraws: 1}).Error)

	draw, status, err := DrawLottery(1202, "draw-request-1")
	require.NoError(t, err)
	assert.Zero(t, status.AvailableDraws)
	assert.Equal(t, int64(1), status.TotalDraws)

	replayed, replayedStatus, err := DrawLottery(1202, "draw-request-1")
	require.NoError(t, err)
	assert.Equal(t, draw.Id, replayed.Id)
	assert.Equal(t, draw.RewardQuota, replayed.RewardQuota)
	assert.Zero(t, replayedStatus.AvailableDraws)

	var user User
	require.NoError(t, DB.First(&user, 1202).Error)
	assert.Equal(t, draw.RewardQuota, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&LotteryDraw{}).Where("user_id = ?", 1202).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestLotteryFiftiethDrawUsesJackpotPool(t *testing.T) {
	truncateTables(t)
	configureLotteryTest(t, true)
	insertLotteryUser(t, 1203)
	require.NoError(t, DB.Create(&LotteryAccount{UserId: 1203, AvailableDraws: 1, TotalDraws: 49}).Error)

	draw, status, err := DrawLottery(1203, "draw-request-50")
	require.NoError(t, err)
	assert.Equal(t, "jackpot", draw.Tier)
	assert.Equal(t, int64(50), draw.DrawNumber)
	assert.Contains(t, []int64{500, 1000, 5000, 10000}, draw.RewardCents)
	assert.False(t, status.NextDrawIsJackpot)
}

func TestLotteryInactiveDoesNotGrantOrDraw(t *testing.T) {
	truncateTables(t)
	configureLotteryTest(t, false)
	insertLotteryUser(t, 1204)
	insertLotteryTopUp(t, 1204, "lottery-disabled", 50, PaymentProviderAlipay, PaymentMethodAlipayNative)
	require.NoError(t, CompleteAlipayTopUp("lottery-disabled", "50.00", "127.0.0.1"))

	status, err := GetLotteryStatus(1204)
	require.NoError(t, err)
	assert.Zero(t, status.AvailableDraws)
	_, _, err = DrawLottery(1204, "disabled-draw")
	assert.ErrorIs(t, err, ErrLotteryInactive)
}

func TestLotteryIgnoresAdministratorCompletedTopUp(t *testing.T) {
	truncateTables(t)
	configureLotteryTest(t, true)
	insertLotteryUser(t, 1205)
	insertLotteryTopUp(t, 1205, "lottery-admin-complete", 50, PaymentProviderEpay, "alipay")

	require.NoError(t, ManualCompleteTopUp("lottery-admin-complete", "127.0.0.1"))
	status, err := GetLotteryStatus(1205)
	require.NoError(t, err)
	assert.Zero(t, status.AvailableDraws)
	assert.Zero(t, status.TotalEligibleRechargeCents)

	var grants int64
	require.NoError(t, DB.Model(&LotteryGrant{}).Count(&grants).Error)
	assert.Zero(t, grants)
}
