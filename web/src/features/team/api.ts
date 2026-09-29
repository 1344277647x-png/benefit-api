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

import type { TeamData, TeamPlan, TeamToken } from './types'

interface Response<T> {
  success: boolean
  message?: string
  data: T
}

async function data<T>(request: Promise<{ data: Response<T> }>): Promise<T> {
  const response = (await request).data
  if (!response.success) throw new Error(response.message || 'Request failed')
  return response.data
}

export const teamApi = {
  self: () => data<TeamData>(api.get('/api/team/self')),
  myUsage: () =>
    data<{ amount: number; requests: number }>(api.get('/api/team/usage')),
  plans: () => data<TeamPlan[]>(api.get('/api/team/plans')),
  create: (name: string) => data(api.post('/api/team/', { name })),
  invite: (email: string) => data(api.post('/api/team/invitations', { email })),
  respond: (id: number, accept: boolean) =>
    data(api.post(`/api/team/invitations/${id}/respond`, { accept })),
  cancel: (id: number) => data(api.delete(`/api/team/invitations/${id}`)),
  remove: (id: number) => data(api.delete(`/api/team/members/${id}`)),
  tokens: () => data<TeamToken[]>(api.get('/api/team/tokens')),
  createToken: (name: string) =>
    data<{ id: number; name: string; key: string }>(
      api.post('/api/team/tokens', { name })
    ),
  disableToken: (id: number) => data(api.delete(`/api/team/tokens/${id}`)),
  balancePay: (planId: number) =>
    data(api.post('/api/team/balance/pay', { plan_id: planId })),
  epay: async (planId: number, method: string) => {
    const response = (
      await api.post('/api/team/epay/pay', {
        plan_id: planId,
        payment_method: method,
      })
    ).data as {
      message: string
      url?: string
      data?: Record<string, string>
    }
    if (response.message !== 'success' || !response.url || !response.data) {
      throw new Error('Payment request failed')
    }
    return response
  },
  resumeEpay: async () => {
    const response = (await api.post('/api/team/epay/resume')).data as {
      message: string
      url?: string
      data?: Record<string, string>
    }
    if (response.message !== 'success' || !response.url || !response.data) {
      throw new Error('Payment request failed')
    }
    return response
  },
  cancelPendingPayment: () => data(api.delete('/api/team/orders/pending')),
  planDissolution: () =>
    data<{ dissolve_at: number }>(api.post('/api/team/dissolution')),
  revokeDissolution: () => data(api.delete('/api/team/dissolution')),
}
