package model

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureReferralTest(t *testing.T, mutate func(*operation_setting.ReferralSetting)) {
	t.Helper()
	rules := operation_setting.GetReferralSetting()
	originalRules := *rules
	payment := operation_setting.GetPaymentSetting()
	originalPayment := *payment
	originalQuotaPerUnit := common.QuotaPerUnit

	common.QuotaPerUnit = 100
	*rules = operation_setting.ReferralSetting{
		Enabled:               true,
		MinimumTopupQuota:     500,
		RewardRateBasisPoints: 500,
		InviteeBonusQuota:     100,
		PerInviteeCapQuota:    1000,
		MonthlyCapQuota:       10000,
		SettlementDelayHours:  24,
	}
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	if mutate != nil {
		mutate(rules)
	}
	t.Cleanup(func() {
		*rules = originalRules
		*payment = originalPayment
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func insertReferralUsers(t *testing.T) {
	t.Helper()
	users := []User{
		{Id: 901, Username: "referral_inviter", AffCode: "r901", Status: common.UserStatusEnabled},
		{Id: 902, Username: "referral_invitee", AffCode: "r902", Status: common.UserStatusEnabled, InviterId: 901},
	}
	require.NoError(t, DB.Create(&users).Error)
}

func insertReferralTopUp(t *testing.T, tradeNo string, amount int64) {
	t.Helper()
	topUp := TopUp{
		UserId:          902,
		Amount:          amount,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodAlipayNative,
		PaymentProvider: PaymentProviderAlipay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, topUp.Insert())
}

func getReferralRewardForTest(t *testing.T) ReferralReward {
	t.Helper()
	var reward ReferralReward
	require.NoError(t, DB.First(&reward).Error)
	return reward
}

func TestReferralRewardCreatedOnFirstQualifyingTopup(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-first-topup", 10)

	require.NoError(t, CompleteAlipayTopUp("referral-first-topup", "9.99", "127.0.0.1"))
	reward := getReferralRewardForTest(t)
	assert.Equal(t, 50, reward.CalculatedRewardQuota)
	assert.Equal(t, 50, reward.RewardQuota)
	assert.Equal(t, 100, reward.InviteeBonusQuota)
	assert.Equal(t, ReferralRewardStatusPending, reward.Status)
	assert.Equal(t, 1100, getUserQuotaForPaymentGuardTest(t, 902))

	var inviter User
	require.NoError(t, DB.First(&inviter, 901).Error)
	assert.Zero(t, inviter.AffQuota)
	assert.Zero(t, inviter.AffHistoryQuota)
}

func TestReferralRewardRequiresEnabledCompliantFirstTopup(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*operation_setting.ReferralSetting)
		setup  func(*testing.T)
		amount int64
	}{
		{
			name: "disabled",
			mutate: func(rules *operation_setting.ReferralSetting) {
				rules.Enabled = false
			},
			amount: 10,
		},
		{
			name:   "below minimum",
			amount: 4,
		},
		{
			name:   "not first successful topup",
			amount: 10,
			setup: func(t *testing.T) {
				previous := TopUp{
					UserId: 902, Amount: 1, Money: 1, TradeNo: "referral-previous-topup",
					PaymentMethod: PaymentMethodAlipayNative, PaymentProvider: PaymentProviderAlipay,
					Status: common.TopUpStatusSuccess, CreateTime: common.GetTimestamp(), CompleteTime: common.GetTimestamp(),
				}
				require.NoError(t, previous.Insert())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			configureReferralTest(t, test.mutate)
			insertReferralUsers(t)
			if test.setup != nil {
				test.setup(t)
			}
			insertReferralTopUp(t, "referral-ineligible-"+test.name, test.amount)

			require.NoError(t, CompleteAlipayTopUp("referral-ineligible-"+test.name, "9.99", "127.0.0.1"))
			var count int64
			require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestReferralTopupCallbackIsIdempotent(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-idempotent", 10)

	require.NoError(t, CompleteAlipayTopUp("referral-idempotent", "9.99", "127.0.0.1"))
	require.NoError(t, CompleteAlipayTopUp("referral-idempotent", "9.99", "127.0.0.1"))
	var count int64
	require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, 1100, getUserQuotaForPaymentGuardTest(t, 902))
}

func TestReferralRewardAppliesPerInviteeAndMonthlyCaps(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, func(rules *operation_setting.ReferralSetting) {
		rules.PerInviteeCapQuota = 30
		rules.MonthlyCapQuota = 100
	})
	insertReferralUsers(t)
	require.NoError(t, DB.Create(&User{Id: 903, Username: "prior_invitee", AffCode: "r903", Status: common.UserStatusEnabled, InviterId: 901}).Error)
	prior := ReferralReward{
		InviterId: 901, InviteeId: 903, TopUpId: 9901, TradeNo: "prior-reward",
		TopUpQuota: 1000, RewardRateBasisPoints: 500, CalculatedRewardQuota: 95,
		RewardQuota: 95, Status: ReferralRewardStatusSettled, CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&prior).Error)
	insertReferralTopUp(t, "referral-capped", 10)

	require.NoError(t, CompleteAlipayTopUp("referral-capped", "9.99", "127.0.0.1"))
	var reward ReferralReward
	require.NoError(t, DB.Where("invitee_id = ?", 902).First(&reward).Error)
	assert.Equal(t, 50, reward.CalculatedRewardQuota)
	assert.Equal(t, 5, reward.RewardQuota)
}

func TestReferralTransferSettlesMaturedRewardsOnce(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, func(rules *operation_setting.ReferralSetting) {
		rules.RewardRateBasisPoints = 5000
	})
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-transfer", 10)
	require.NoError(t, CompleteAlipayTopUp("referral-transfer", "9.99", "127.0.0.1"))
	require.NoError(t, DB.Model(&ReferralReward{}).Where("invitee_id = ?", 902).Update("available_at", common.GetTimestamp()-1).Error)

	require.NoError(t, TransferReferralQuota(901, 100))
	var inviter User
	require.NoError(t, DB.First(&inviter, 901).Error)
	assert.Equal(t, 100, inviter.Quota)
	assert.Equal(t, 400, inviter.AffQuota)
	assert.Equal(t, 500, inviter.AffHistoryQuota)
	assert.Equal(t, ReferralRewardStatusSettled, getReferralRewardForTest(t).Status)

	overview, err := GetReferralOverview(901)
	require.NoError(t, err)
	assert.Equal(t, 400, overview.AvailableQuota)
	assert.Equal(t, 500, overview.TotalRewardQuota)
}

func TestReferralRewardRequiresPaymentCompliance(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-no-compliance", 10)

	require.NoError(t, CompleteAlipayTopUp("referral-no-compliance", "9.99", "127.0.0.1"))
	var count int64
	require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, 1000, getUserQuotaForPaymentGuardTest(t, 902))
}

func TestReferralInvalidInviterNeverBlocksPaidTopup(t *testing.T) {
	tests := []struct {
		name      string
		inviterId int
	}{
		{name: "no inviter", inviterId: 0},
		{name: "self inviter", inviterId: 902},
		{name: "deleted inviter", inviterId: 999},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			configureReferralTest(t, nil)
			require.NoError(t, DB.Create(&User{
				Id: 902, Username: "referral_invitee", AffCode: "r902", Status: common.UserStatusEnabled, InviterId: test.inviterId,
			}).Error)
			insertReferralTopUp(t, "referral-invalid-inviter-"+test.name, 10)

			require.NoError(t, CompleteAlipayTopUp("referral-invalid-inviter-"+test.name, "9.99", "127.0.0.1"))
			var count int64
			require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
			assert.Zero(t, count)
			assert.Equal(t, 1000, getUserQuotaForPaymentGuardTest(t, 902))
		})
	}
}

func TestReferralSecondTopupDoesNotQualifyAfterLowFirstTopup(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-low-first", 4)
	require.NoError(t, CompleteAlipayTopUp("referral-low-first", "9.99", "127.0.0.1"))
	insertReferralTopUp(t, "referral-high-second", 10)
	require.NoError(t, CompleteAlipayTopUp("referral-high-second", "9.99", "127.0.0.1"))

	var count int64
	require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, 1400, getUserQuotaForPaymentGuardTest(t, 902))
}

func TestReferralRewardRunsThroughEveryTopupCompletionPath(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		method   string
		amount   int64
		money    float64
		complete func(string) error
	}{
		{name: "alipay", provider: PaymentProviderAlipay, method: PaymentMethodAlipayNative, amount: 10, money: 10, complete: func(tradeNo string) error {
			return CompleteAlipayTopUp(tradeNo, "10.00", "127.0.0.1")
		}},
		{name: "epay", provider: PaymentProviderEpay, method: "alipay", amount: 10, money: 10, complete: func(tradeNo string) error {
			return CompleteEpayTopUp(tradeNo, "alipay", "127.0.0.1")
		}},
		{name: "stripe", provider: PaymentProviderStripe, method: PaymentMethodStripe, amount: 10, money: 10, complete: func(tradeNo string) error {
			return Recharge(tradeNo, "cus_referral", "127.0.0.1")
		}},
		{name: "creem", provider: PaymentProviderCreem, method: PaymentMethodCreem, amount: 1000, money: 10, complete: func(tradeNo string) error {
			return RechargeCreem(tradeNo, "", "", "127.0.0.1")
		}},
		{name: "waffo", provider: PaymentProviderWaffo, method: PaymentMethodWaffo, amount: 10, money: 10, complete: func(tradeNo string) error {
			return RechargeWaffo(tradeNo, "127.0.0.1")
		}},
		{name: "waffo pancake", provider: PaymentProviderWaffoPancake, method: PaymentMethodWaffoPancake, amount: 10, money: 10, complete: func(tradeNo string) error {
			return RechargeWaffoPancake(tradeNo)
		}},
		{name: "manual completion", provider: PaymentProviderEpay, method: "alipay", amount: 10, money: 10, complete: func(tradeNo string) error {
			return ManualCompleteTopUp(tradeNo, "127.0.0.1")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			configureReferralTest(t, nil)
			insertReferralUsers(t)
			tradeNo := "referral-provider-" + test.name
			require.NoError(t, (&TopUp{
				UserId: 902, Amount: test.amount, Money: test.money, TradeNo: tradeNo,
				PaymentMethod: test.method, PaymentProvider: test.provider,
				Status: common.TopUpStatusPending, CreateTime: common.GetTimestamp(),
			}).Insert())

			require.NoError(t, test.complete(tradeNo))
			reward := getReferralRewardForTest(t)
			assert.Equal(t, 1000, reward.TopUpQuota)
			assert.Equal(t, 50, reward.RewardQuota)
			assert.Equal(t, 100, reward.InviteeBonusQuota)
			assert.Equal(t, 1100, getUserQuotaForPaymentGuardTest(t, 902))
		})
	}
}

func TestReferralRewardsAreCappedBeforeQuotaFieldsOverflow(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	users := []User{
		{Id: 901, Username: "referral_inviter", AffCode: "r901", Status: common.UserStatusEnabled, AffHistoryQuota: common.MaxQuota - 20},
		{Id: 902, Username: "referral_invitee", AffCode: "r902", Status: common.UserStatusEnabled, InviterId: 901, Quota: common.MaxQuota - 1050},
	}
	require.NoError(t, DB.Create(&users).Error)
	insertReferralTopUp(t, "referral-overflow-cap", 10)

	require.NoError(t, CompleteAlipayTopUp("referral-overflow-cap", "9.99", "127.0.0.1"))
	reward := getReferralRewardForTest(t)
	assert.Equal(t, 20, reward.RewardQuota)
	assert.Equal(t, 50, reward.InviteeBonusQuota)
	assert.Equal(t, common.MaxQuota, getUserQuotaForPaymentGuardTest(t, 902))
}

func TestReferralQuotaConversionRejectsInvalidAndOverflowingValues(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	_, err := quotaFromTopupAmount(0)
	require.Error(t, err)
	_, err = quotaFromTopupAmount(-1)
	require.Error(t, err)
	_, err = quotaFromTopupAmount(math.MaxInt64)
	require.Error(t, err)
	_, err = quotaFromTopupMoney(math.MaxFloat64)
	require.Error(t, err)
}

func TestReferralMonthRangeUsesShanghaiNaturalMonth(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	moment := time.Date(2026, time.March, 31, 23, 59, 59, 0, location)
	start, end := referralMonthRange(moment.Unix())
	assert.Equal(t, time.Date(2026, time.March, 1, 0, 0, 0, 0, location).Unix(), start)
	assert.Equal(t, time.Date(2026, time.April, 1, 0, 0, 0, 0, location).Unix(), end)
}

func TestReferralConcurrentCallbacksOnlyCreditOnce(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	insertReferralUsers(t)
	insertReferralTopUp(t, "referral-concurrent-callback", 10)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- CompleteAlipayTopUp("referral-concurrent-callback", "9.99", "127.0.0.1")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, DB.Model(&ReferralReward{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, 1100, getUserQuotaForPaymentGuardTest(t, 902))
}

func TestReferralConcurrentTransfersCannotOverspend(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	require.NoError(t, DB.Create(&User{
		Id: 901, Username: "referral_inviter", AffCode: "r901", Status: common.UserStatusEnabled, AffQuota: 100,
	}).Error)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- TransferReferralQuota(901, 100)
		}()
	}
	wg.Wait()
	close(errs)
	var successes int
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)

	var inviter User
	require.NoError(t, DB.First(&inviter, 901).Error)
	assert.Equal(t, 100, inviter.Quota)
	assert.Zero(t, inviter.AffQuota)
}

func TestReferralOptionsArePersistedAndAppliedTogether(t *testing.T) {
	truncateTables(t)
	configureReferralTest(t, nil)
	values := map[string]string{
		"referral_setting.enabled":                  "true",
		"referral_setting.minimum_topup_quota":      "1000",
		"referral_setting.reward_rate_basis_points": "300",
		"referral_setting.invitee_bonus_quota":      "50",
		"referral_setting.per_invitee_cap_quota":    "500",
		"referral_setting.monthly_cap_quota":        "5000",
		"referral_setting.settlement_delay_hours":   "72",
	}

	require.NoError(t, UpdateOptionsBulkWithActivationKey(values, "referral_setting.enabled"))
	rules := operation_setting.GetReferralSettingSnapshot()
	assert.True(t, rules.Enabled)
	assert.Equal(t, 1000, rules.MinimumTopupQuota)
	assert.Equal(t, 300, rules.RewardRateBasisPoints)
	assert.Equal(t, 50, rules.InviteeBonusQuota)
	assert.Equal(t, 500, rules.PerInviteeCapQuota)
	assert.Equal(t, 5000, rules.MonthlyCapQuota)
	assert.Equal(t, 72, rules.SettlementDelayHours)

	var stored []Option
	require.NoError(t, DB.Where("key LIKE ?", "referral_setting.%").Find(&stored).Error)
	assert.Len(t, stored, len(values))
}
