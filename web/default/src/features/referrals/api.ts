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
