import { api } from '@/lib/api'

import type {
  ApiResponse,
  LotteryDrawResult,
  LotteryHistory,
  LotteryStatus,
} from './types'

export async function getLotteryStatus() {
  const response = await api.get<ApiResponse<LotteryStatus>>(
    '/api/lottery/status'
  )
  return response.data
}

export async function drawLottery(requestKey: string) {
  const response = await api.post<ApiResponse<LotteryDrawResult>>(
    '/api/lottery/draw',
    null,
    { headers: { 'Idempotency-Key': requestKey } }
  )
  return response.data
}

export async function getLotteryHistory(page = 1, pageSize = 20) {
  const response = await api.get<ApiResponse<LotteryHistory>>(
    '/api/lottery/history',
    { params: { p: page, page_size: pageSize } }
  )
  return response.data
}
