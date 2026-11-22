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
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  normalizeImageBatchMode,
  parseImageBatchModelModes,
} from '../channel-form'

describe('channel image batch settings', () => {
  test('normalizes unknown modes and parses model overrides without duplicates', () => {
    expect(normalizeImageBatchMode('fanout')).toBe('fanout')
    expect(normalizeImageBatchMode('unsupported')).toBe('native')
    expect(
      parseImageBatchModelModes({
        'gpt-image-2': 'fanout',
        'upstream-image': 'native',
      })
    ).toEqual([
      { model: 'gpt-image-2', mode: 'fanout' },
      { model: 'upstream-image', mode: 'native' },
    ])
  })

  test('serializes channel default and model overrides into setting JSON', () => {
    const setting = JSON.parse(
      buildSettingJSON({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        image_batch_mode: 'native',
        image_batch_model_modes: [{ model: 'gpt-image-2', mode: 'fanout' }],
      })
    )

    expect(setting.image_batch_mode).toBeUndefined()
    expect(setting.image_batch_model_modes).toEqual({
      'gpt-image-2': 'fanout',
    })
  })

  test('rejects empty and duplicate model overrides', () => {
    const empty = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      image_batch_model_modes: [{ model: '  ', mode: 'fanout' }],
    })
    const duplicate = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      image_batch_model_modes: [
        { model: 'gpt-image-2', mode: 'fanout' },
        { model: ' gpt-image-2 ', mode: 'native' },
      ],
    })

    expect(empty.success).toBe(false)
    expect(duplicate.success).toBe(false)
  })
})
