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
  CreationApiResponse,
  CreationImagePayload,
  CreationJobsPage,
  CreationModelsData,
  CreationVideoPayload,
  GenerationAsset,
  GenerationJob,
} from './types'

export async function getCreationModels(): Promise<
  CreationApiResponse<CreationModelsData>
> {
  const response = await api.get('/api/creation/models')
  return response.data
}

export async function uploadCreationAsset(
  file: File
): Promise<CreationApiResponse<GenerationAsset>> {
  const form = new FormData()
  form.append('file', file, file.name)
  const response = await api.post('/api/creation/uploads', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
    skipErrorHandler: true,
  })
  return response.data
}

export async function createImage(
  payload: CreationImagePayload
): Promise<CreationApiResponse<GenerationJob>> {
  const response = await api.post('/pg/creation/images', payload, {
    skipErrorHandler: true,
  })
  return response.data
}

export async function createVideo(
  payload: CreationVideoPayload
): Promise<CreationApiResponse<GenerationJob>> {
  const response = await api.post('/pg/creation/videos', payload, {
    skipErrorHandler: true,
  })
  return response.data
}

export async function getCreationJobs(
  page = 1,
  pageSize = 24
): Promise<CreationApiResponse<CreationJobsPage>> {
  const response = await api.get('/api/creation/jobs', {
    params: { p: page, page_size: pageSize },
  })
  return response.data
}

export async function getCreationJob(
  id: string
): Promise<CreationApiResponse<GenerationJob>> {
  const response = await api.get(`/api/creation/jobs/${encodeURIComponent(id)}`)
  return response.data
}

export async function deleteCreationJob(
  id: string
): Promise<CreationApiResponse<{ id: string }>> {
  const response = await api.delete(
    `/api/creation/jobs/${encodeURIComponent(id)}`
  )
  return response.data
}

export async function retryCreationArchive(
  id: string
): Promise<CreationApiResponse<GenerationJob>> {
  const response = await api.post(
    `/api/creation/jobs/${encodeURIComponent(id)}/retry-archive`
  )
  return response.data
}
