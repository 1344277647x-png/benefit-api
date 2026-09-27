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
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { parse, type Rule } from 'postcss'
import { describe, expect, it } from 'vitest'

const css = parse(
  readFileSync(resolve('src/styles/cinematic-home.css'), 'utf8')
)

function declarations(selector: string, media?: string) {
  const result: Record<string, string> = {}
  css.walkRules((rule: Rule) => {
    if (!rule.selectors.includes(selector)) return
    const parent = rule.parent
    const query = parent?.type === 'atrule' ? parent.params : undefined
    if (query !== media) return
    rule.walkDecls((decl) => {
      result[decl.prop] = decl.value
    })
  })
  return result
}

describe('cinematic homepage layout contracts', () => {
  it('keeps the full-hero cosmic scenery below content and removes the large foreground on phones', () => {
    const cosmos = declarations('.benefit-cinema-cosmos')
    expect(cosmos.position).toBe('absolute')
    expect(cosmos.inset).toBe('0')
    expect(cosmos['pointer-events']).toBe('none')
    expect(cosmos.overflow).toBe('hidden')
    expect(declarations('.benefit-cinema-hero-grid')['z-index']).toBe('1')
    expect(
      declarations('.benefit-cinema-foreground-planet', '(max-width: 767px)')
        .display
    ).toBe('none')
    expect(
      declarations('.benefit-cinema-galaxy-distant', '(max-width: 767px)').top
    ).toBe('82px')
    expect(
      declarations('.benefit-cinema-galaxy-near', '(max-width: 767px)').bottom
    ).toBe('82px')
  })
  it('uses shrinkable two-column tracks on desktop and a single column on phones', () => {
    expect(
      declarations('.benefit-cinema-hero-grid')['grid-template-columns']
    ).toBe('minmax(0, 1.05fr) minmax(0, 1fr)')
    expect(
      declarations('.benefit-cinema-hero-grid', '(max-width: 767px)')[
        'grid-template-columns'
      ]
    ).toBe('minmax(0, 1fr)')
    expect(declarations('.benefit-cinema-copy')['min-width']).toBe('0')
    expect(
      declarations('.benefit-cinema-visual', '(max-width: 767px)').width
    ).toBe('100%')
  })

  it('lets long translated action labels wrap without shrinking the touch target', () => {
    const buttons = declarations('.benefit-cinema-actions > *')
    expect(buttons['min-height']).toBe('48px')
    expect(buttons.height).toBe('auto')
    expect(buttons['white-space']).toBe('normal')
    expect(declarations('.benefit-cinema-actions')['flex-wrap']).toBe('wrap')
  })

  it('provides a static decorative halo even when the GPU cannot render', () => {
    const halo = declarations('.benefit-cinema-halo')
    expect(halo['aspect-ratio']).toBe('1')
    expect(halo['border-radius']).toBe('50%')
    expect(halo.background).toContain('radial-gradient')
    expect(declarations('.benefit-cinema-visual')['pointer-events']).toBe(
      'none'
    )
  })

  it('removes motion and reveals scroll-animated copy in reduced-motion mode', () => {
    const animations = declarations(
      '.benefit-cinema-home *',
      '(prefers-reduced-motion: reduce)'
    )
    expect(animations.animation).toBe('none')
    expect(animations.transition).toBe('none')
    const reduced: string[] = []
    css.walkAtRules('media', (media) => {
      if (media.params !== '(prefers-reduced-motion: reduce)') return
      media.walkDecls('opacity', (decl) => {
        reduced.push(decl.value)
      })
    })
    expect(reduced).toContain('1')
  })

  it('scopes every custom selector to the homepage so administrative tables are untouched', () => {
    css.walkRules((rule) => {
      if (rule.parent?.type === 'atrule' && rule.parent.name === 'keyframes') {
        return
      }
      for (const selector of rule.selectors) {
        expect(selector).toMatch(/^(\.light )?\.benefit-cinema-/)
      }
    })
  })

  it('contains decorative model nodes on narrow screens and wraps long protocol paths', () => {
    expect(
      declarations('.benefit-cinema-node', '(max-width: 767px)').width
    ).toBe('31%')
    expect(declarations('.benefit-cinema-node')['min-width']).toBe('0')
    expect(declarations('.benefit-cinema-node-protocol')['overflow-wrap']).toBe(
      'anywhere'
    )
    expect(declarations('.benefit-cinema-star-map').width).toBe('100%')
    expect(declarations('.benefit-cinema-star-map').height).toBe('100%')
    expect(
      declarations('.benefit-cinema-visual .benefit-hero-3d-stage').inset
    ).toBe('12% 10%')
  })

  it('pauses star and orbit animations by default and enables them only in the visible hero', () => {
    expect(declarations('.benefit-cinema-hero')['--cosmic-play-state']).toBe(
      'paused'
    )
    expect(
      declarations(".benefit-cinema-hero[data-decorative-motion='running']")[
        '--cosmic-play-state'
      ]
    ).toBe('running')
    for (const selector of [
      '.benefit-cinema-stars circle',
      '.benefit-cinema-deep-stars',
      '.benefit-cinema-galaxy-arms',
    ]) {
      expect(declarations(selector)['animation-play-state']).toBe(
        'var(--cosmic-play-state, paused)'
      )
    }
    expect(
      declarations(
        '.benefit-cinema-node-surface',
        '(prefers-reduced-motion: reduce)'
      ).transform
    ).toBe('none')
    css.walkAtRules('keyframes', (rule) => {
      expect(rule.params).toMatch(/^benefit-cinema-/)
    })
  })
})
