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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { createImage, deleteCreationUpload, uploadCreationAssets } from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    delete: vi.fn(),
    post: vi.fn(),
  },
}))

const mockedPost = vi.mocked(api.post)
const mockedDelete = vi.mocked(api.delete)

afterEach(() => {
  mockedPost.mockReset()
  mockedDelete.mockReset()
})

describe('creation batch API', () => {
  test('uploads every reference image under the repeated files field', async () => {
    mockedPost.mockResolvedValue({
      data: { success: true, data: { assets: [] } },
    })
    const files = [
      new File(['first'], 'first.png', { type: 'image/png' }),
      new File(['second'], 'second.webp', { type: 'image/webp' }),
    ]

    await uploadCreationAssets(files)

    expect(mockedPost).toHaveBeenCalledOnce()
    const [path, form] = mockedPost.mock.calls[0]
    expect(path).toBe('/api/creation/uploads/batch')
    expect(form).toBeInstanceOf(FormData)
    expect((form as FormData).getAll('files')).toEqual(files)
  })

  test('submits count four and all uploaded asset ids in one image request', async () => {
    mockedPost.mockResolvedValue({
      data: { success: true, data: { id: 'gen_1' } },
    })

    await createImage({
      model: 'gpt-image-2',
      protocol: 'openai-image',
      prompt: 'consistent campaign images',
      count: 4,
      reference_asset_ids: ['asset_first', 'asset_second'],
    })

    expect(mockedPost).toHaveBeenCalledWith(
      '/pg/creation/images',
      expect.objectContaining({
        count: 4,
        reference_asset_ids: ['asset_first', 'asset_second'],
      }),
      expect.objectContaining({ skipErrorHandler: true })
    )
  })

  test('deletes an unbound uploaded asset through the owner-scoped endpoint', async () => {
    mockedDelete.mockResolvedValue({
      data: { success: true, data: { id: 'asset_orphan' } },
    })

    await deleteCreationUpload('asset_orphan')

    expect(mockedDelete).toHaveBeenCalledWith(
      '/api/creation/uploads/asset_orphan',
      { skipErrorHandler: true }
    )
  })
})
