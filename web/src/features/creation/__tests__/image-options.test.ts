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
  getInitialImageAspectRatio,
  getInitialImageResolution,
  getResolvedImageSize,
} from '../image-options'

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

describe('Image2 resolution options', () => {
  const sizePresets = {
    '1K': {
      '1:1': '1024x1024',
      '3:2': '1536x1024',
      '2:3': '1024x1536',
      '16:9': '1280x720',
      '9:16': '720x1280',
      '4:3': '1024x768',
      '3:4': '768x1024',
      '21:9': '1280x544',
    },
    '2K': {
      '1:1': '2048x2048',
      '3:2': '2160x1440',
      '2:3': '1440x2160',
      '16:9': '2560x1440',
      '9:16': '1440x2560',
      '4:3': '2048x1536',
      '3:4': '1536x2048',
      '21:9': '2560x1088',
    },
    '4K': {
      '1:1': '2880x2880',
      '3:2': '3456x2304',
      '2:3': '2304x3456',
      '16:9': '3840x2160',
      '9:16': '2160x3840',
      '4:3': '3200x2400',
      '3:4': '2400x3200',
      '21:9': '3840x1600',
    },
  }
  const capabilities = {
    reference_image: true,
    aspect_ratios: ['1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9'],
    resolution_tiers: ['1K', '2K', '4K'],
    default_resolution: '4K',
    size_presets: sizePresets,
  }

  test('uses the server-provided 4K default', () => {
    expect(getInitialImageResolution(capabilities)).toBe('4K')
  })

  test('reads horizontal and vertical sizes from server capabilities', () => {
    expect(getResolvedImageSize(capabilities, '4K', '16:9')).toBe('3840x2160')
    expect(getResolvedImageSize(capabilities, '4K', '9:16')).toBe('2160x3840')
  })

  test('reads all 24 server-provided resolution and aspect-ratio combinations', () => {
    for (const [resolution, sizes] of Object.entries(sizePresets)) {
      for (const [aspectRatio, expectedSize] of Object.entries(sizes)) {
        expect(
          getResolvedImageSize(capabilities, resolution, aspectRatio)
        ).toBe(expectedSize)
      }
    }
  })

  test('clears the image resolution for non-Image2 capabilities', () => {
    expect(getInitialImageResolution({ reference_image: true })).toBe('')
    expect(getResolvedImageSize({ reference_image: true }, '4K', '16:9')).toBe(
      ''
    )
  })
})
