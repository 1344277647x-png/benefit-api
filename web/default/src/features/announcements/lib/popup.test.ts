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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PopupStorage, SystemAnnouncement } from '../types'
import {
  dismissPopupAnnouncements,
  getAnnouncementVersionKey,
  getPendingPopupAnnouncements,
  shouldShowAnnouncementPopupOnPath,
} from './popup'

class MemoryStorage implements PopupStorage {
  private values = new Map<string, string>()

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }
}

const publishedAt = new Date(2026, 7, 19, 8).toISOString()
const now = new Date(2026, 7, 19, 12)

function announcement(
  frequency: SystemAnnouncement['popupFrequency'],
  overrides: Partial<SystemAnnouncement> = {}
): SystemAnnouncement {
  return {
    id: 1,
    content: 'Service update',
    publishDate: publishedAt,
    type: 'default',
    popupEnabled: true,
    popupFrequency: frequency,
    ...overrides,
  }
}

describe('announcement popup policy', () => {
  test('keeps legacy announcements disabled and filters future announcements', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const pending = getPendingPopupAnnouncements(
      [
        announcement('once', { id: 1, popupEnabled: undefined }),
        announcement('once', {
          id: 2,
          publishDate: new Date(2026, 7, 20, 8).toISOString(),
        }),
        announcement('once', { id: 3, publishDate: publishedAt }),
      ],
      now,
      localStorage,
      sessionStorage
    )

    assert.deepEqual(
      pending.map((item) => item.id),
      [3]
    )
  })

  test('once dismissal survives later days in the same browser', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const item = announcement('once')

    dismissPopupAnnouncements([item], now, localStorage, sessionStorage)

    assert.equal(
      getPendingPopupAnnouncements(
        [item],
        new Date(2026, 7, 20, 12),
        localStorage,
        new MemoryStorage()
      ).length,
      0
    )
  })

  test('daily dismissal expires on the next local day', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const item = announcement('daily')

    dismissPopupAnnouncements([item], now, localStorage, sessionStorage)

    assert.equal(
      getPendingPopupAnnouncements(
        [item],
        new Date(2026, 7, 19, 23),
        localStorage,
        sessionStorage
      ).length,
      0
    )
    assert.equal(
      getPendingPopupAnnouncements(
        [item],
        new Date(2026, 7, 20, 0, 1),
        localStorage,
        sessionStorage
      ).length,
      1
    )
  })

  test('session dismissal expires with session storage', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const item = announcement('session')

    dismissPopupAnnouncements([item], now, localStorage, sessionStorage)

    assert.equal(
      getPendingPopupAnnouncements([item], now, localStorage, sessionStorage)
        .length,
      0
    )
    assert.equal(
      getPendingPopupAnnouncements(
        [item],
        now,
        localStorage,
        new MemoryStorage()
      ).length,
      1
    )
  })

  test('editing announcement content creates a new popup version', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const original = announcement('once')
    const edited = { ...original, content: 'Updated service notice' }

    dismissPopupAnnouncements([original], now, localStorage, sessionStorage)

    assert.notEqual(
      getAnnouncementVersionKey(original),
      getAnnouncementVersionKey(edited)
    )
    assert.deepEqual(
      getPendingPopupAnnouncements(
        [original, edited],
        now,
        localStorage,
        sessionStorage
      ).map((item) => item.content),
      ['Updated service notice']
    )
  })

  test('combines current announcements newest first without using deleted cache', () => {
    const localStorage = new MemoryStorage()
    const sessionStorage = new MemoryStorage()
    const older = announcement('once', {
      id: 1,
      publishDate: new Date(2026, 7, 18, 8).toISOString(),
    })
    const newer = announcement('once', { id: 2 })

    assert.deepEqual(
      getPendingPopupAnnouncements(
        [older, newer],
        now,
        localStorage,
        sessionStorage
      ).map((item) => item.id),
      [2, 1]
    )
    assert.deepEqual(
      getPendingPopupAnnouncements([], now, localStorage, sessionStorage),
      []
    )
  })

  test('excludes setup, OAuth callback, and error routes', () => {
    assert.equal(shouldShowAnnouncementPopupOnPath('/'), true)
    assert.equal(shouldShowAnnouncementPopupOnPath('/sign-in'), true)
    assert.equal(shouldShowAnnouncementPopupOnPath('/setup'), false)
    assert.equal(shouldShowAnnouncementPopupOnPath('/oauth/github'), false)
    assert.equal(shouldShowAnnouncementPopupOnPath('/500'), false)
    assert.equal(
      shouldShowAnnouncementPopupOnPath('/errors/internal-server-error'),
      false
    )
  })
})
