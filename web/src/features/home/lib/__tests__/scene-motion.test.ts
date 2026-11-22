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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { startSceneMotion } from '../scene-motion'

let frames: Map<number, FrameRequestCallback>
let nextFrame: number
let onIntersection: IntersectionObserverCallback
let media: MediaQueryList
let host: HTMLDivElement
let stop: (() => void) | undefined
const disconnect = vi.fn()

function setVisible(visible: boolean) {
  onIntersection(
    [{ isIntersecting: visible } as IntersectionObserverEntry],
    {} as IntersectionObserver
  )
}

function tick(time: number) {
  const pending = [...frames.values()]
  frames.clear()
  pending.forEach((frame) => frame(time))
}

beforeEach(() => {
  frames = new Map()
  nextFrame = 0
  host = document.createElement('div')
  media = Object.assign(new EventTarget(), { matches: false }) as MediaQueryList
  vi.spyOn(window, 'matchMedia').mockReturnValue(media)
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frames.set(++nextFrame, callback)
    return nextFrame
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    frames.delete(id)
  })
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        onIntersection = callback
      }
      observe() {}
      disconnect = disconnect
    }
  )
})

afterEach(() => {
  stop?.()
  stop = undefined
  vi.unstubAllGlobals()
})

describe('decorative scene motion', () => {
  it('starts only when visible and cancels frames when scrolled away', () => {
    stop = startSceneMotion(host, vi.fn())
    expect(frames.size).toBe(0)
    setVisible(true)
    expect(host.dataset.motionState).toBe('running')
    expect(frames.size).toBe(1)
    setVisible(false)
    expect(host.dataset.motionState).toBe('paused')
    expect(frames.size).toBe(0)
  })

  it('caps rendering to 30 FPS even on a high refresh-rate screen', () => {
    const render = vi.fn()
    stop = startSceneMotion(host, render)
    setVisible(true)
    tick(0)
    tick(8)
    tick(16)
    tick(24)
    expect(render).not.toHaveBeenCalled()
    tick(34)
    expect(render).toHaveBeenCalledExactlyOnceWith(34)
  })

  it('renders a static frame for reduced motion and responds to live preference changes', () => {
    Object.defineProperty(media, 'matches', { value: true, writable: true })
    const render = vi.fn()
    stop = startSceneMotion(host, render)
    setVisible(true)
    expect(render).toHaveBeenLastCalledWith(0)
    expect(frames.size).toBe(0)
    Object.assign(media, { matches: false })
    media.dispatchEvent(new Event('change'))
    expect(frames.size).toBe(1)
    Object.assign(media, { matches: true })
    media.dispatchEvent(new Event('change'))
    expect(host.dataset.motionState).toBe('reduced')
    expect(frames.size).toBe(0)
  })

  it('pauses background tabs and resumes without jumping forward in animation time', () => {
    const render = vi.fn()
    stop = startSceneMotion(host, render)
    setVisible(true)
    tick(0)
    tick(40)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(0)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    tick(10000)
    tick(10040)
    expect(render).toHaveBeenLastCalledWith(80)
  })

  it('removes animation and listeners on unmount, even if an observer callback was queued', () => {
    stop = startSceneMotion(host, vi.fn())
    setVisible(true)
    stop()
    expect(frames.size).toBe(0)
    expect(disconnect).toHaveBeenCalled()
    setVisible(true)
    media.dispatchEvent(new Event('change'))
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(0)
  })
})
