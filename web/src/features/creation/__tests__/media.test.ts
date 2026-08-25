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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { getFreshAuthHeaders } from '@/lib/api'

import { downloadAuthenticatedAsset, getAssetAccessEndpoint } from '../media'

vi.mock('@/lib/api', () => ({
  getFreshAuthHeaders: vi.fn(),
}))

const mockedGetFreshAuthHeaders = vi.mocked(getFreshAuthHeaders)

afterEach(() => {
  vi.restoreAllMocks()
  mockedGetFreshAuthHeaders.mockReset()
})

describe('creation media access', () => {
  test('maps a content URL to the signed access endpoint without preserving query data', () => {
    expect(
      getAssetAccessEndpoint(
        '/api/creation/assets/asset_123/content?download=1'
      )
    ).toBe('http://localhost:3000/api/creation/assets/asset_123/access')
  })

  test('rejects an external content URL before attaching authentication headers', () => {
    expect(() =>
      getAssetAccessEndpoint(
        'https://attacker.example/assets/asset_123/content'
      )
    ).toThrow('Invalid generation asset URL')
  })

  test('downloads through a short-lived signed URL and never sends the asset as a blob', async () => {
    mockedGetFreshAuthHeaders.mockResolvedValue({
      Authorization: 'Bearer access-token',
    })
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          data: {
            url: '/api/creation/assets/asset_123/content?ticket=short-lived',
            expires_at: 1_900_000_000,
          },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    vi.stubGlobal('fetch', fetchMock)
    const click = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(click)

    await downloadAuthenticatedAsset(
      '/api/creation/assets/asset_123/content',
      'asset_123'
    )

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:3000/api/creation/assets/asset_123/access?download=1',
      expect.objectContaining({
        credentials: 'include',
        headers: { Authorization: 'Bearer access-token' },
      })
    )
    expect(click).toHaveBeenCalledOnce()
  })
})
