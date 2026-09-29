export type LotteryPrize = {
  reward_cents: number
  weight: number
}

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
  rule_version: string
  regular_pool: LotteryPrize[]
  jackpot_pool: LotteryPrize[]
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
