/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useState } from 'react'

import { getFreshAuthHeaders } from '@/lib/api'

type AuthenticatedAssetState = {
  url: string
  loading: boolean
  failed: boolean
}

type AssetAccessResponse = {
  success: boolean
  message?: string
  data?: {
    url: string
    expires_at: number
  }
}

export function getAssetAccessEndpoint(sourceUrl: string): string {
  const parsed = new URL(sourceUrl, window.location.origin)
  if (
    parsed.origin !== window.location.origin ||
    !/^\/api\/creation\/assets\/[^/]+\/content$/.test(parsed.pathname)
  ) {
    throw new Error('Invalid generation asset URL')
  }
  parsed.pathname = `${parsed.pathname.slice(0, -'/content'.length)}/access`
  parsed.search = ''
  return parsed.toString()
}

async function fetchAssetAccess(
  url: string,
  signal: AbortSignal,
  download = false
): Promise<string> {
  const endpoint = getAssetAccessEndpoint(url)
  let headers = await getFreshAuthHeaders()
  const requestUrl = download ? `${endpoint}?download=1` : endpoint
  let response = await fetch(requestUrl, {
    credentials: 'include',
    headers,
    signal,
  })
  if (response.status === 401 && !signal.aborted) {
    headers = await getFreshAuthHeaders()
    response = await fetch(requestUrl, {
      credentials: 'include',
      headers,
      signal,
    })
  }
  if (!response.ok) {
    throw new Error(`asset access request failed: ${response.status}`)
  }
  const payload = (await response.json()) as AssetAccessResponse
  if (!payload.success || !payload.data?.url) {
    throw new Error(payload.message || 'asset access request failed')
  }
  return payload.data.url
}

export function useAuthenticatedAssetUrl(
  sourceUrl: string
): AuthenticatedAssetState {
  const [state, setState] = useState<AuthenticatedAssetState>({
    url: '',
    loading: Boolean(sourceUrl),
    failed: false,
  })

  useEffect(() => {
    if (!sourceUrl) {
      setState({ url: '', loading: false, failed: false })
      return
    }

    const controller = new AbortController()
    setState({ url: '', loading: true, failed: false })

    void (async () => {
      try {
        const assetUrl = await fetchAssetAccess(sourceUrl, controller.signal)
        if (controller.signal.aborted) return
        setState({ url: assetUrl, loading: false, failed: false })
      } catch {
        if (!controller.signal.aborted) {
          setState({ url: '', loading: false, failed: true })
        }
      }
    })()

    return () => {
      controller.abort()
    }
  }, [sourceUrl])

  return state
}

export async function downloadAuthenticatedAsset(
  sourceUrl: string,
  filename: string
): Promise<void> {
  const controller = new AbortController()
  try {
    const assetUrl = await fetchAssetAccess(sourceUrl, controller.signal, true)
    const anchor = document.createElement('a')
    anchor.href = assetUrl
    anchor.download = filename
    anchor.rel = 'noreferrer'
    anchor.style.display = 'none'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
  } finally {
    controller.abort()
  }
}
