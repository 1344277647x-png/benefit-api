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
import { describe, expect, test } from 'vitest'

import {
  appendReferenceImageFiles,
  MAX_REFERENCE_IMAGE_BYTES,
  MAX_REFERENCE_IMAGE_TOTAL_BYTES,
} from '../reference-image-files'

function imageFile(name: string, type = 'image/png', size = 8): File {
  return new File([new Uint8Array(size)], name, {
    type,
    lastModified: 1_900_000_000_000 + size,
  })
}

describe('creation reference image selection', () => {
  test('keeps selection order when four valid images are added at once', () => {
    const selected = [
      imageFile('first.png'),
      imageFile('second.png'),
      imageFile('third.png'),
      imageFile('fourth.png'),
    ]

    const result = appendReferenceImageFiles([], selected, 4)

    expect(result.files.map((file) => file.name)).toEqual([
      'first.png',
      'second.png',
      'third.png',
      'fourth.png',
    ])
    expect(result.overflowCount).toBe(0)
  })

  test('rejects the fifth valid image without disturbing the first four', () => {
    const current = [
      imageFile('first.png'),
      imageFile('second.png'),
      imageFile('third.png'),
      imageFile('fourth.png'),
    ]

    const result = appendReferenceImageFiles(
      current,
      [imageFile('fifth.png')],
      4
    )

    expect(result.files).toEqual(current)
    expect(result.overflowCount).toBe(1)
  })

  test('rejects oversized and unsupported files while accepting valid files', () => {
    const result = appendReferenceImageFiles(
      [],
      [
        imageFile('valid.webp', 'image/webp'),
        imageFile('oversized.jpg', 'image/jpeg', MAX_REFERENCE_IMAGE_BYTES + 1),
        imageFile('vector.svg', 'image/svg+xml'),
      ],
      4
    )

    expect(result.files.map((file) => file.name)).toEqual(['valid.webp'])
    expect(result.oversizedCount).toBe(1)
    expect(result.unsupportedCount).toBe(1)
  })

  test('ignores the same local file when it is selected again', () => {
    const original = imageFile('same.png')

    const result = appendReferenceImageFiles(
      [original],
      [imageFile('same.png')],
      4
    )

    expect(result.files).toEqual([original])
    expect(result.overflowCount).toBe(0)
  })

  test('rejects files that exceed the aggregate reference size limit', () => {
    const result = appendReferenceImageFiles(
      [],
      [
        imageFile('first.png', 'image/png', 15 * 1024 * 1024),
        imageFile('second.png', 'image/png', 15 * 1024 * 1024),
        imageFile('third.png', 'image/png', 15 * 1024 * 1024),
      ],
      4,
      MAX_REFERENCE_IMAGE_BYTES,
      MAX_REFERENCE_IMAGE_TOTAL_BYTES
    )

    expect(result.files.map((file) => file.name)).toEqual([
      'first.png',
      'second.png',
    ])
    expect(result.totalSizeExceededCount).toBe(1)
  })
})
