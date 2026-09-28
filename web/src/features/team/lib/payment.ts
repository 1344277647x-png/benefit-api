import type { PaymentMethod, TopupInfo } from '@/features/wallet/types'

// A team order supports Epay only. The wallet's combined pay_methods list
// also includes native Alipay and other gateways with different callbacks.
export function availableTeamEpayMethods(
  payment: TopupInfo | undefined
): PaymentMethod[] {
  if (!payment?.enable_online_topup) return []
  return (payment.epay_pay_methods || []).filter((method) => !!method.type)
}
