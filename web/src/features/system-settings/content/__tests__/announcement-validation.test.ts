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
  announcementSchema,
  countAnnouncementCharacters,
  MAX_ANNOUNCEMENT_CONTENT_CHARACTERS,
} from '../announcement-validation'

const validAnnouncement = {
  content: 'notice',
  publishDate: '2026-09-23T00:00:00Z',
  type: 'default' as const,
  extra: '',
  popupEnabled: true,
  popupFrequency: 'once' as const,
}

describe('announcement content validation', () => {
  test('accepts a Markdown announcement longer than the legacy 500-character limit', () => {
    const result = announcementSchema.safeParse({
      ...validAnnouncement,
      content: `# Update\n\n${'Configuration details\n'.repeat(100)}`,
    })

    expect(result.success).toBe(true)
  })

  test('accepts the new limit and rejects one character beyond it', () => {
    expect(
      announcementSchema.safeParse({
        ...validAnnouncement,
        content: 'a'.repeat(MAX_ANNOUNCEMENT_CONTENT_CHARACTERS),
      }).success
    ).toBe(true)
    expect(
      announcementSchema.safeParse({
        ...validAnnouncement,
        content: 'a'.repeat(MAX_ANNOUNCEMENT_CONTENT_CHARACTERS + 1),
      }).success
    ).toBe(false)
  })

  test('counts text the same way as the browser maxLength attribute', () => {
    expect(countAnnouncementCharacters('通知😀')).toBe('通知😀'.length)
  })
})
