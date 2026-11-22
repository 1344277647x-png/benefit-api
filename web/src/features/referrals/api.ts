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
import { api } from '@/lib/api'

import type {
  ReferralInviteesResponse,
  ReferralOverviewResponse,
  ReferralRewardsResponse,
} from './types'

export async function getReferralOverview(): Promise<ReferralOverviewResponse> {
  const res = await api.get('/api/user/referral/overview')
  return res.data
}

export async function getReferralRewards(
  page = 1,
  pageSize = 50
): Promise<ReferralRewardsResponse> {
  const res = await api.get('/api/user/referral/rewards', {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function getReferralInvitees(
  page = 1,
  pageSize = 50
): Promise<ReferralInviteesResponse> {
  const res = await api.get('/api/user/referral/invitees', {
    params: { p: page, page_size: pageSize },
  })
  return res.data
}

export async function transferReferralQuota(quota: number) {
  const res = await api.post('/api/user/referral/transfer', { quota })
  return res.data as { success?: boolean; message?: string }
}
