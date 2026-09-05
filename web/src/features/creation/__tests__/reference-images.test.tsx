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
import { useRef, useState, type ChangeEvent } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { appendReferenceImageFiles } from '../reference-image-files'
import { ReferenceImagePicker } from '../reference-images'

const t = (key: string, options?: Record<string, unknown>) => {
  let value = key
  for (const [name, replacement] of Object.entries(options ?? {})) {
    value = value.replace(`{{${name}}}`, String(replacement))
  }
  return value
}

function PickerHarness() {
  const [files, setFiles] = useState<File[]>([
    new File(['first'], 'first.png', { type: 'image/png', lastModified: 1 }),
    new File(['second'], 'second.png', {
      type: 'image/png',
      lastModified: 2,
    }),
    new File(['third'], 'third.png', { type: 'image/png', lastModified: 3 }),
  ])
  const inputRef = useRef<HTMLInputElement>(null)
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setFiles(
      (current) =>
        appendReferenceImageFiles(current, [...(event.target.files ?? [])], 4)
          .files
    )
  }
  return (
    <ReferenceImagePicker
      files={files}
      maxFiles={4}
      maxTotalBytes={40 * 1024 * 1024}
      inputRef={inputRef}
      onChange={handleChange}
      onRemove={(index) =>
        setFiles((current) =>
          current.filter((_, currentIndex) => currentIndex !== index)
        )
      }
      onClear={() => setFiles([])}
      t={t}
    />
  )
}

beforeEach(() => {
  vi.spyOn(URL, 'createObjectURL').mockImplementation(
    (file) => `blob:${(file as File).name}`
  )
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('creation reference image picker', () => {
  test('supports multiple selection and removes the middle file without reordering the rest', () => {
    render(<PickerHarness />)
    const input = screen.getByLabelText('Choose reference images')
    expect(input).toHaveAttribute('multiple')

    fireEvent.click(
      screen.getByRole('button', { name: 'Remove reference image 2' })
    )

    const names = screen
      .getByTestId('reference-image-picker')
      .querySelectorAll('p.truncate')
    expect([...names].map((node) => node.textContent)).toEqual([
      'first.png',
      'third.png',
    ])
  })

  test('clears all selected reference images with a keyboard-accessible button', () => {
    render(<PickerHarness />)
    const clearButton = screen.getByRole('button', { name: 'Clear all' })

    clearButton.focus()
    fireEvent.keyDown(clearButton, { key: 'Enter' })
    fireEvent.click(clearButton)

    expect(screen.queryByText('first.png')).not.toBeInTheDocument()
    expect(screen.getByText('0/4')).toBeInTheDocument()
  })
})
