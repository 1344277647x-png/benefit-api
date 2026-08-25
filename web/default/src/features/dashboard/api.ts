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
  FlowQuotaDataItem,
  OperationsReport,
  OperationsReportGroupBy,
  QuotaDataItem,
  UptimeGroupResult,
} from './types'

// ============================================================================
// Dashboard APIs
// ============================================================================

// ----------------------------------------------------------------------------
// Quota & Usage Data
// ----------------------------------------------------------------------------

// Get user quota data within a time range
// Admin users get all users' data by default (matching classic frontend behavior)
export async function getUserQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data' : '/api/data/self'
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    endpoint,
    { params }
  )
  return res.data
}

// ----------------------------------------------------------------------------
// System Monitoring
// ----------------------------------------------------------------------------

export async function getUserQuotaDataByUsers(params: {
  start_timestamp: number
  end_timestamp: number
}) {
  const res = await api.get<{ success: boolean; data: QuotaDataItem[] }>(
    '/api/data/users',
    { params }
  )
  return res.data
}

export async function getFlowQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    default_time?: string
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data/flow' : '/api/data/flow/self'
  const res = await api.get<{
    success: boolean
    data?: FlowQuotaDataItem[]
    message?: string
  }>(endpoint, { params })
  return res.data
}

// Get uptime monitoring status for all services
export async function getUptimeStatus() {
  const res = await api.get<{ success: boolean; data: UptimeGroupResult[] }>(
    '/api/uptime/status'
  )
  return res.data
}

export async function getOperationsReport(params: {
  start_timestamp: number
  end_timestamp: number
  group_by: OperationsReportGroupBy
  model_name?: string
  username?: string
  group?: string
  channel_id?: number
}) {
  const res = await api.get<{
    success: boolean
    message?: string
    data?: OperationsReport
  }>('/api/log/operations-report', { params })
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Unable to load operations report')
  }
  return res.data
}

export async function downloadOperationsReport(params: {
  start_timestamp: number
  end_timestamp: number
  group_by: OperationsReportGroupBy
  model_name?: string
  username?: string
  group?: string
  channel_id?: number
}) {
  const res = await api.get<Blob>('/api/log/operations-report', {
    params: { ...params, format: 'csv' },
    responseType: 'blob',
  })
  const contentType = String(res.headers['content-type'] ?? '').toLowerCase()
  if (!contentType.includes('text/csv')) {
    const payload = await res.data.text()
    let message: string | undefined
    try {
      const parsed = JSON.parse(payload) as { message?: string }
      message = parsed.message
    } catch {
      // Keep malformed error bodies from leaking parser details into the UI.
    }
    throw new Error(message || 'Unable to load operations report')
  }
  return res.data
}
