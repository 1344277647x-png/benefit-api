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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { observeHeroMotion } from '../hero-motion'

let host: HTMLElement
let media: MediaQueryList
let intersection: IntersectionObserverCallback
let stop: () => void
const disconnect = vi.fn()

beforeEach(() => {
  host = document.createElement('section')
  media = Object.assign(new EventTarget(), { matches: false }) as MediaQueryList
  vi.spyOn(window, 'matchMedia').mockReturnValue(media)
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersection = callback
      }
      observe() {}
      disconnect = disconnect
    }
  )
  stop = observeHeroMotion(host)
})

afterEach(() => {
  stop()
  vi.unstubAllGlobals()
})

function show(visible: boolean) {
  intersection(
    [{ isIntersecting: visible } as IntersectionObserverEntry],
    {} as IntersectionObserver
  )
}

it('runs CSS animations only while the hero is onscreen and pauses when scrolled away', () => {
  expect(host.dataset.decorativeMotion).toBe('paused')
  show(true)
  expect(host.dataset.decorativeMotion).toBe('running')
  show(false)
  expect(host.dataset.decorativeMotion).toBe('paused')
})

it('pauses background tabs, clears hover effects, and resumes when visible again', () => {
  const card = document.createElement('div')
  card.dataset.hovered = 'true'
  host.append(card)
  show(true)
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
  document.dispatchEvent(new Event('visibilitychange'))
  expect(host.dataset.decorativeMotion).toBe('paused')
  expect(card).not.toHaveAttribute('data-hovered')
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  document.dispatchEvent(new Event('visibilitychange'))
  expect(host.dataset.decorativeMotion).toBe('running')
})

it('responds immediately to reduced-motion preference changes', () => {
  show(true)
  Object.assign(media, { matches: true })
  media.dispatchEvent(new Event('change'))
  expect(host.dataset.decorativeMotion).toBe('reduced')
  Object.assign(media, { matches: false })
  media.dispatchEvent(new Event('change'))
  expect(host.dataset.decorativeMotion).toBe('running')
})

it('does not restart CSS motion after disposal even if an observer callback was queued', () => {
  show(true)
  stop()
  show(true)
  document.dispatchEvent(new Event('visibilitychange'))
  expect(host.dataset.decorativeMotion).toBe('paused')
  expect(disconnect).toHaveBeenCalled()
})
