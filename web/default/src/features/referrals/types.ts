import type { ApiResponse } from '@/features/wallet/types'

export type ReferralRules = {
  enabled: boolean
  minimum_topup_quota: number
  reward_rate_basis_points: number
  invitee_bonus_quota: number
  per_invitee_cap_quota: number
  monthly_cap_quota: number
  settlement_delay_hours: number
}

export type ReferralOverview = {
  enabled: boolean
  compliance_ready: boolean
  affiliate_code: string
  invite_count: number
  qualified_count: number
  pending_quota: number
  available_quota: number
  total_reward_quota: number
  rules: ReferralRules
}

export type ReferralRewardItem = {
  invitee_username: string
  reward_quota: number
  status: string
  available_at: number
  created_at: number
}

export type ReferralInviteeItem = {
  username: string
  created_at: number
  qualified: boolean
  reward_status: string
}

export type ReferralPage<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type ReferralOverviewResponse = ApiResponse<ReferralOverview>
export type ReferralRewardsResponse = ApiResponse<
  ReferralPage<ReferralRewardItem>
>
export type ReferralInviteesResponse = ApiResponse<
  ReferralPage<ReferralInviteeItem>
>
