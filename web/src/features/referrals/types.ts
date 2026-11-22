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
