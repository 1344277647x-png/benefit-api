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
import type {
  AnnouncementPopupFrequency,
  PopupStorage,
  SystemAnnouncement,
} from '../types'

const POPUP_ONCE_STORAGE_KEY = 'announcement-popup-dismissed-once'
const POPUP_DAILY_STORAGE_KEY = 'announcement-popup-dismissed-daily'
const POPUP_SESSION_STORAGE_KEY = 'announcement-popup-dismissed-session'

function hashString(input: string): string {
  let hash = 0
  for (let index = 0; index < input.length; index += 1) {
    hash = (hash << 5) - hash + input.charCodeAt(index)
    hash |= 0
  }
  return hash.toString(36)
}

function getAnnouncementFingerprint(item: SystemAnnouncement): string {
  return JSON.stringify({
    id: item.id ?? '',
    publishDate: item.publishDate ?? '',
    content: item.content.trim(),
    extra: item.extra?.trim() ?? '',
    type: item.type ?? '',
  })
}

export function getAnnouncementVersionKey(item: SystemAnnouncement): string {
  return `version:${String(item.id ?? 'none')}:${hashString(getAnnouncementFingerprint(item))}`
}

export function getAnnouncementReadKey(item: SystemAnnouncement): string {
  if (item.id !== undefined && item.id !== null) {
    return `id:${item.id}`
  }
  return `hash:${hashString(
    JSON.stringify({
      publishDate: item.publishDate ?? '',
      content: item.content.trim(),
      extra: item.extra?.trim() ?? '',
      type: item.type ?? '',
      title: item.title?.trim() ?? '',
      link: item.link?.trim() ?? '',
    })
  )}`
}

function readStringSet(storage: PopupStorage, key: string): Set<string> {
  try {
    const value = JSON.parse(storage.getItem(key) ?? '[]') as unknown
    if (!Array.isArray(value)) return new Set()
    return new Set(
      value.filter((item): item is string => typeof item === 'string')
    )
  } catch {
    return new Set()
  }
}

function writeStringSet(
  storage: PopupStorage,
  key: string,
  values: Set<string>
): void {
  try {
    storage.setItem(key, JSON.stringify([...values]))
  } catch {
    /* Browsers may disable storage in private or restricted contexts. */
  }
}

function readDailyDismissals(storage: PopupStorage): Record<string, string> {
  try {
    const value = JSON.parse(storage.getItem(POPUP_DAILY_STORAGE_KEY) ?? '{}')
    if (!value || typeof value !== 'object' || Array.isArray(value)) return {}

    return Object.fromEntries(
      Object.entries(value).filter(
        (entry): entry is [string, string] => typeof entry[1] === 'string'
      )
    )
  } catch {
    return {}
  }
}

function getLocalDateKey(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function getPopupFrequency(
  item: SystemAnnouncement
): AnnouncementPopupFrequency {
  if (item.popupFrequency === 'daily' || item.popupFrequency === 'session') {
    return item.popupFrequency
  }
  return 'once'
}

export function getPendingPopupAnnouncements(
  announcements: SystemAnnouncement[],
  now: Date,
  localStorage: PopupStorage,
  sessionStorage: PopupStorage
): SystemAnnouncement[] {
  const onceDismissals = readStringSet(localStorage, POPUP_ONCE_STORAGE_KEY)
  const dailyDismissals = readDailyDismissals(localStorage)
  const sessionDismissals = readStringSet(
    sessionStorage,
    POPUP_SESSION_STORAGE_KEY
  )
  const today = getLocalDateKey(now)
  const seen = new Set<string>()

  return announcements
    .filter((item) => {
      if (item.popupEnabled !== true || !item.content.trim()) return false

      const publishTimestamp = Date.parse(item.publishDate ?? '')
      if (
        !Number.isFinite(publishTimestamp) ||
        publishTimestamp > now.getTime()
      ) {
        return false
      }

      const versionKey = getAnnouncementVersionKey(item)
      if (seen.has(versionKey)) return false
      seen.add(versionKey)

      const frequency = getPopupFrequency(item)
      if (frequency === 'daily') {
        return dailyDismissals[versionKey] !== today
      }
      if (frequency === 'session') {
        return !sessionDismissals.has(versionKey)
      }
      return !onceDismissals.has(versionKey)
    })
    .sort(
      (left, right) =>
        Date.parse(right.publishDate ?? '') - Date.parse(left.publishDate ?? '')
    )
}

export function dismissPopupAnnouncements(
  announcements: SystemAnnouncement[],
  now: Date,
  localStorage: PopupStorage,
  sessionStorage: PopupStorage
): void {
  const onceDismissals = readStringSet(localStorage, POPUP_ONCE_STORAGE_KEY)
  const dailyDismissals = readDailyDismissals(localStorage)
  const sessionDismissals = readStringSet(
    sessionStorage,
    POPUP_SESSION_STORAGE_KEY
  )
  const today = getLocalDateKey(now)

  for (const item of announcements) {
    const versionKey = getAnnouncementVersionKey(item)
    const frequency = getPopupFrequency(item)
    if (frequency === 'daily') {
      dailyDismissals[versionKey] = today
    } else if (frequency === 'session') {
      sessionDismissals.add(versionKey)
    } else {
      onceDismissals.add(versionKey)
    }
  }

  writeStringSet(localStorage, POPUP_ONCE_STORAGE_KEY, onceDismissals)
  try {
    localStorage.setItem(
      POPUP_DAILY_STORAGE_KEY,
      JSON.stringify(dailyDismissals)
    )
  } catch {
    /* Browsers may disable storage in private or restricted contexts. */
  }
  writeStringSet(sessionStorage, POPUP_SESSION_STORAGE_KEY, sessionDismissals)
}

export function shouldShowAnnouncementPopupOnPath(pathname: string): boolean {
  if (pathname === '/setup' || pathname.startsWith('/setup/')) return false
  if (pathname === '/oauth' || pathname.startsWith('/oauth/')) return false
  if (pathname === '/errors' || pathname.startsWith('/errors/')) return false
  return !/^\/(401|403|404|500|503)\/?$/.test(pathname)
}
