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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { CreationFormFields } from '../index'

function formProps() {
  return {
    models: [],
    modelId: '',
    setModelId: vi.fn(),
    group: 'default',
    setGroup: vi.fn(),
    groups: ['default', 'Codex Plus'],
    prompt: '',
    setPrompt: vi.fn(),
    count: '1',
    setCount: vi.fn(),
    size: '',
    setSize: vi.fn(),
    aspectRatio: '1:1',
    setAspectRatio: vi.fn(),
    quality: '',
    setQuality: vi.fn(),
    duration: '4',
    setDuration: vi.fn(),
    imageResolution: '4K',
    setImageResolution: vi.fn(),
    videoResolution: '720p',
    setVideoResolution: vi.fn(),
    t: (key: string) => key,
  }
}

describe('Creation form labels', () => {
  it('associates model, group and image option labels with their controls', () => {
    const props = formProps()
    render(
      <CreationFormFields
        {...props}
        kind='image'
        capabilities={{
          reference_image: true,
          max_count: 4,
          sizes: ['1024x1024'],
          aspect_ratios: ['1:1', '16:9'],
        }}
      />
    )
    expect(screen.getByRole('combobox', { name: 'Model' })).toBeDisabled()
    expect(
      screen.getByRole('combobox', { name: 'Channel group' })
    ).toBeDisabled()
    const aspect = screen.getByRole('combobox', { name: 'Aspect ratio' })
    fireEvent.change(aspect, { target: { value: '16:9' } })
    expect(props.setAspectRatio).toHaveBeenCalledWith('16:9')
  })

  it('associates video duration and resolution labels with separate controls', () => {
    render(<CreationFormFields {...formProps()} kind='video' />)
    expect(
      screen.getByRole('combobox', { name: 'Duration (seconds)' })
    ).toHaveValue('4')
    expect(screen.getByRole('combobox', { name: 'Resolution' })).toHaveValue(
      '720p'
    )
  })
})
