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
import { render, screen, within } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { ActiveJobCard } from '../index'
import type { GenerationAsset, GenerationJob } from '../types'

vi.mock('../media', () => ({
  useAuthenticatedAssetUrl: (sourceUrl: string) => ({
    url: `${sourceUrl}?ticket=test`,
    loading: false,
    failed: false,
  }),
  downloadAuthenticatedAsset: vi.fn().mockResolvedValue(undefined),
}))

const t = (key: string, options?: Record<string, unknown>) => {
  let value = key
  for (const [name, replacement] of Object.entries(options ?? {})) {
    value = value.replace(`{{${name}}}`, String(replacement))
  }
  return value
}

function asset(id: string, role: 'input' | 'output'): GenerationAsset {
  return {
    id,
    role,
    mime_type: 'image/png',
    size_bytes: 100,
    status: 'ready',
    created_at: 1,
    expires_at: 2,
    content_url: `/api/creation/assets/${id}/content`,
  }
}

describe('creation result gallery', () => {
  test('separates input references and renders every output with its own download', () => {
    const job: GenerationJob = {
      id: 'gen_1',
      kind: 'image',
      protocol: 'openai-image',
      model: 'gpt-image-2',
      prompt: 'campaign set',
      status: 'succeeded',
      requested_count: 4,
      result_count: 3,
      created_at: 1,
      updated_at: 1,
      expires_at: 2,
      assets: [
        asset('input_1', 'input'),
        asset('input_2', 'input'),
        asset('output_1', 'output'),
        asset('output_2', 'output'),
        asset('output_3', 'output'),
      ],
    }

    render(<ActiveJobCard job={job} t={t} onRetry={vi.fn()} />)

    const inputs = screen.getByRole('region', {
      name: 'Input reference images',
    })
    const outputs = screen.getByRole('region', { name: 'Generated results' })
    expect(within(inputs).getAllByRole('img')).toHaveLength(2)
    expect(within(outputs).getAllByRole('img')).toHaveLength(3)
    expect(
      within(outputs).getAllByRole('button', { name: /Download result/ })
    ).toHaveLength(3)
    expect(screen.getByText('3 generated images')).toBeInTheDocument()
    expect(
      screen.getByText(
        'The model returned 3 of 4 requested images. Billing follows actual upstream usage.'
      )
    ).toBeInTheDocument()
  })
})
