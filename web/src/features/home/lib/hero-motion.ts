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

/** Gate CSS animations without adding an animation frame loop. */
export function observeHeroMotion(host: HTMLElement): () => void {
  const media = window.matchMedia('(prefers-reduced-motion: reduce)')
  let visible = false
  let disposed = false
  const sync = () => {
    if (disposed) return
    let state = 'paused'
    if (media.matches) state = 'reduced'
    else if (visible && !document.hidden) state = 'running'
    host.dataset.decorativeMotion = state
    if (state !== 'running') {
      host.querySelectorAll<HTMLElement>('[data-hovered]').forEach((card) => {
        delete card.dataset.hovered
      })
    }
  }
  const observer = new IntersectionObserver(([entry]) => {
    visible = entry.isIntersecting
    sync()
  })
  observer.observe(host)
  media.addEventListener('change', sync)
  document.addEventListener('visibilitychange', sync)
  sync()
  return () => {
    disposed = true
    observer.disconnect()
    media.removeEventListener('change', sync)
    document.removeEventListener('visibilitychange', sync)
    host.dataset.decorativeMotion = 'paused'
  }
}
