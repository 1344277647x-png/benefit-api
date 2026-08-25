/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

export type IntegrationProtocol =
  | 'openai-chat'
  | 'openai-responses'
  | 'anthropic'
  | 'gemini'
  | 'image'
  | 'gemini-image'
  | 'video'
  | 'embeddings'
  | 'rerank'

export type IntegrationModel = {
  modelName: string
  vendorName?: string
  protocols: IntegrationProtocol[]
  endpointTypes: string[]
}

export const endpointProtocol: Record<string, IntegrationProtocol> = {
  openai: 'openai-chat',
  'openai-response': 'openai-responses',
  'openai-response-compact': 'openai-responses',
  anthropic: 'anthropic',
  gemini: 'gemini',
  'gemini-image': 'gemini-image',
  'image-generation': 'image',
  'openai-video': 'video',
  embeddings: 'embeddings',
  'jina-rerank': 'rerank',
}

type PricingPayload = {
  success?: boolean
  data?: Array<{
    model_name?: unknown
    vendor_name?: unknown
    supported_endpoint_types?: unknown
  }>
}

function normalizeEndpointTypes(raw: unknown): string[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter((value): value is string => typeof value === 'string')
    .map((value) => value.trim())
    .filter(Boolean)
}

function toIntegrationModel(item: {
  model_name?: unknown
  vendor_name?: unknown
  supported_endpoint_types?: unknown
}): IntegrationModel | null {
  const modelName =
    typeof item.model_name === 'string' ? item.model_name.trim() : ''
  if (!modelName) return null

  const endpointTypes = normalizeEndpointTypes(item.supported_endpoint_types)
  const protocols = endpointTypes
    .map((endpoint) => endpointProtocol[endpoint])
    .filter(
      (protocol, index, values): protocol is IntegrationProtocol =>
        Boolean(protocol) && values.indexOf(protocol) === index
    )

  return {
    modelName,
    vendorName:
      typeof item.vendor_name === 'string'
        ? item.vendor_name.trim()
        : undefined,
    protocols,
    endpointTypes,
  }
}

function uniqueModels(models: IntegrationModel[]): IntegrationModel[] {
  const seen = new Set<string>()
  return models.filter((model) => {
    const key = model.modelName.toLowerCase()
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export async function getIntegrationModels(): Promise<IntegrationModel[]> {
  const isAuthenticated = Boolean(useAuthStore.getState().auth.user)

  // Prefer the authenticated catalog: unlike the public pricing page, it is
  // filtered by this user's usable groups and enabled channel capabilities.
  if (isAuthenticated) {
    try {
      const response = await api.get<{
        success?: boolean
        data?: Array<{
          model_name?: unknown
          vendor_name?: unknown
          supported_endpoint_types?: unknown
        }>
      }>('/api/user/integration-models', { skipErrorHandler: true } as Record<
        string,
        unknown
      >)
      if (
        response.data?.success !== false &&
        Array.isArray(response.data?.data)
      ) {
        const models = response.data.data
          .map(toIntegrationModel)
          .filter((model): model is IntegrationModel => model !== null)
        return uniqueModels(models)
      }
      // An authenticated catalog failure must fail closed. Falling back to
      // the public pricing catalog could show capabilities from another user
      // group while the scoped endpoint is unavailable.
      return []
    } catch {
      return []
    }
  }

  try {
    const response = await api.get<PricingPayload>('/api/pricing', {
      skipErrorHandler: true,
    } as Record<string, unknown>)
    const payload = response.data
    if (payload?.success !== false && Array.isArray(payload?.data)) {
      const models = payload.data
        .map(toIntegrationModel)
        .filter((model): model is IntegrationModel => model !== null)
      if (models.length > 0) return uniqueModels(models)
    }
  } catch {
    // The public catalog can be disabled; try the authenticated model list.
  }

  return []
}

export function protocolLabel(protocol: IntegrationProtocol): string {
  return {
    'openai-chat': 'Chat Completions',
    'openai-responses': 'Responses',
    anthropic: 'Claude Messages',
    gemini: 'Gemini',
    image: 'Image generation',
    'gemini-image': 'Gemini image generation',
    video: 'Video generation',
    embeddings: 'Embeddings',
    rerank: 'Rerank',
  }[protocol]
}
