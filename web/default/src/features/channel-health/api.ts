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

export type PublicHealthStatus =
  | 'normal'
  | 'delayed'
  | 'unavailable'
  | 'unknown'

export interface PublicModelHealth {
  model: string
  status: PublicHealthStatus
  last_sample_at?: number
}

export interface PublicHealthResponse {
  success: boolean
  message?: string
  data?: {
    items: PublicModelHealth[]
    refreshed_at: number
  }
}

export interface ChannelHealthView {
  channel_id: number
  model: string
  endpoint_type?: string
  status: PublicHealthStatus
  request_count: number
  success_rate: number
  average_latency_ms: number
  average_ttft_ms: number
  consecutive_failures: number
  last_sample_at: number
  last_success_at: number
  last_error_class?: string
  last_error_code?: string
  last_http_status?: number
}

export interface ChannelHealthResponse {
  success: boolean
  message?: string
  data?: {
    items: ChannelHealthView[]
    refreshed_at: number
  }
}

export async function getPublicChannelHealth(): Promise<PublicHealthResponse> {
  const response = await api.get('/api/channel-health/public')
  return response.data
}

export async function getChannelHealth(): Promise<ChannelHealthResponse> {
  const response = await api.get('/api/channel/health')
  return response.data
}
