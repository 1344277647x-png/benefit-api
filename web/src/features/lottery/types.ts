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
export type LotteryStatus = {
  enabled: boolean
  active: boolean
  start_at: number
  end_at: number
  available_draws: number
  total_draws: number
  next_draw_is_jackpot: boolean
  recharge_remainder_cents: number
  amount_to_next_draw_cents: number
  total_eligible_recharge_cents: number
  total_reward_quota: number
  draw_threshold_cents: number
  jackpot_interval: number
  prize_amounts_cents: number[]
}

export type LotteryDraw = {
  id: number
  user_id: number
  draw_number: number
  tier: 'regular' | 'jackpot'
  reward_cents: number
  reward_quota: number
  rule_version: string
  created_at: number
}

export type LotteryHistory = {
  items: LotteryDraw[]
  page: number
  page_size: number
  total: number
}

export type ApiResponse<T> = {
  success: boolean
  message: string
  data?: T
}

export type LotteryDrawResult = {
  draw: LotteryDraw
  status: LotteryStatus
}
