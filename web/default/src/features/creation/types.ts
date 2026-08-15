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
export type CreationKind = 'image' | 'video'

export type CreationProtocol =
  | 'openai-image'
  | 'imagen'
  | 'gemini-image'
  | 'openai-video'

export interface CreationCapabilities {
  reference_image: boolean
  max_count?: number
  sizes?: string[]
  aspect_ratios?: string[]
  qualities?: string[]
  durations?: number[]
  resolutions?: string[]
}

export interface CreationModel {
  id: string
  display_name: string
  kind: CreationKind
  protocol: CreationProtocol
  groups: string[]
  capabilities: CreationCapabilities
}

export interface GenerationStorageUsage {
  user_bytes: number
  system_bytes: number
  user_limit: number
  system_limit: number
  reserved_bytes: number
}

export interface GenerationAsset {
  id: string
  role: 'input' | 'output' | string
  mime_type: string
  size_bytes: number
  status: string
  created_at: number
  expires_at: number
  content_url?: string
}

export type GenerationJobStatus =
  | 'pending'
  | 'queued'
  | 'processing'
  | 'archiving'
  | 'archive_failed'
  | 'succeeded'
  | 'failed'

export interface GenerationJob {
  id: string
  kind: CreationKind
  protocol: string
  model: string
  prompt: string
  task_id?: string
  status: GenerationJobStatus
  error_code?: string
  error_message?: string
  archive_attempts?: number
  created_at: number
  updated_at: number
  expires_at: number
  assets?: GenerationAsset[]
}

export interface CreationModelsData {
  models: CreationModel[]
  storage: GenerationStorageUsage
  retention_days: number
}

export interface CreationApiResponse<T> {
  success: boolean
  message?: string
  data?: T
}

export interface CreationJobsPage {
  items: GenerationJob[]
  total: number
  page: number
  page_size: number
}

export interface CreationImagePayload {
  model: string
  protocol: CreationProtocol
  group?: string
  prompt: string
  size?: string
  aspect_ratio?: string
  quality?: string
  count: number
  reference_asset_id?: string
}

export interface CreationVideoPayload {
  model: string
  group?: string
  prompt: string
  duration: number
  resolution: string
  reference_asset_id?: string
}
