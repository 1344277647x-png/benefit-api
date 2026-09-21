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

import { getInitialImageAspectRatio } from '../image-options'

describe('creation image aspect ratio defaults', () => {
  test('keeps the ratio automatic when pixel sizes are also available', () => {
    expect(
      getInitialImageAspectRatio({
        reference_image: true,
        sizes: ['1024x1024'],
        aspect_ratios: ['1:1', '3:2'],
      })
    ).toBe('')
  })

  test('uses the first supported ratio for ratio-only image models', () => {
    expect(
      getInitialImageAspectRatio({
        reference_image: true,
        aspect_ratios: ['1:1', '3:2', '21:9'],
      })
    ).toBe('1:1')
  })
})
