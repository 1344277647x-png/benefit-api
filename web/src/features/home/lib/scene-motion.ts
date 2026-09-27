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

/** Decorative WebGL runs at most 30 FPS, and only while it can be seen. */
export function startSceneMotion(
  host: HTMLElement,
  render: (elapsed: number) => void
): () => void {
  const media = window.matchMedia('(prefers-reduced-motion: reduce)')
  let intersecting = false
  let frameId = 0
  let previousTime: number | undefined
  let elapsed = 0
  let disposed = false

  const animate = (time: number) => {
    if (disposed) return
    if (previousTime === undefined) previousTime = time
    const delta = time - previousTime
    if (delta >= 1000 / 30) {
      // Do not fast-forward after backgrounding or a heavily delayed frame.
      elapsed += Math.min(delta, 64)
      previousTime = time
      render(elapsed)
    }
    frameId = window.requestAnimationFrame(animate)
  }

  const sync = () => {
    if (disposed) return
    window.cancelAnimationFrame(frameId)
    previousTime = undefined
    if (media.matches) {
      host.dataset.motionState = 'reduced'
      render(0)
    } else if (intersecting && !document.hidden) {
      host.dataset.motionState = 'running'
      frameId = window.requestAnimationFrame(animate)
    } else {
      host.dataset.motionState = 'paused'
    }
  }

  const observer = new IntersectionObserver(
    ([entry]) => {
      intersecting = entry.isIntersecting
      sync()
    },
    { threshold: 0 }
  )
  observer.observe(host)
  media.addEventListener('change', sync)
  document.addEventListener('visibilitychange', sync)
  sync()

  return () => {
    disposed = true
    window.cancelAnimationFrame(frameId)
    observer.disconnect()
    media.removeEventListener('change', sync)
    document.removeEventListener('visibilitychange', sync)
  }
}
