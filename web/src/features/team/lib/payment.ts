/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { PaymentMethod, TopupInfo } from '@/features/wallet/types'

// A team order supports Epay only. The wallet's combined pay_methods list
// also includes native Alipay and other gateways with different callbacks.
export function availableTeamEpayMethods(
  payment: TopupInfo | undefined
): PaymentMethod[] {
  if (!payment?.enable_online_topup) return []
  return (payment.epay_pay_methods || []).filter((method) => !!method.type)
}
