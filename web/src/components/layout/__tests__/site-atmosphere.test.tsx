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
import { act, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { SiteAtmosphere } from '../components/site-atmosphere'

describe('Site atmosphere accessibility and motion', () => {
  it('pauses when the document is hidden and resumes when visible', () => {
    let visibility: DocumentVisibilityState = 'visible'
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(
      () => visibility
    )
    const view = render(<SiteAtmosphere />)
    const decoration = view.container.firstElementChild
    expect(decoration).toHaveAttribute('aria-hidden', 'true')
    expect(decoration).toHaveAttribute('data-motion', 'running')
    expect(
      view.container.querySelectorAll('button, a, [tabindex]')
    ).toHaveLength(0)

    act(() => {
      visibility = 'hidden'
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(decoration).toHaveAttribute('data-motion', 'paused')
    act(() => {
      visibility = 'visible'
      document.dispatchEvent(new Event('visibilitychange'))
    })
    expect(decoration).toHaveAttribute('data-motion', 'running')
  })

  it('reacts to reduced-motion changes and releases its listener on unmount', () => {
    const media = new EventTarget()
    let reduced = true
    const remove = vi.spyOn(media, 'removeEventListener')
    vi.spyOn(window, 'matchMedia').mockReturnValue(
      Object.assign(media, {
        get matches() {
          return reduced
        },
        media: '(prefers-reduced-motion: reduce)',
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
      }) as MediaQueryList
    )
    // Keep matches live: Object.assign would otherwise copy the getter's value.
    Object.defineProperty(media, 'matches', { get: () => reduced })
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const view = render(<SiteAtmosphere />)
    expect(view.container.firstElementChild).toHaveAttribute(
      'data-motion',
      'paused'
    )
    act(() => {
      reduced = false
      media.dispatchEvent(new Event('change'))
    })
    expect(view.container.firstElementChild).toHaveAttribute(
      'data-motion',
      'running'
    )
    view.unmount()
    expect(remove).toHaveBeenCalledWith('change', expect.any(Function))
  })
})
