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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { parse, type Rule } from 'postcss'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it } from 'vitest'

import { AuthLayout } from '@/features/auth/auth-layout'
import en from '@/i18n/locales/en.json'

function declarations(filePath: string, selector: string, media?: string) {
  const css = parse(readFileSync(resolve(filePath), 'utf8'))
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

describe('sitewide cinematic style contracts', () => {
  it('quiets workspace atmosphere, syncs non-default presets, and unifies card/table polish', () => {
    expect(
      declarations(
        'src/styles/cinematic-site.css',
        'body.benefit-cinematic-site[data-theme-preset]'
      )['--cinema-site-gold']
    ).toBe('var(--primary)')

    for (const selector of [
      'body:has(.benefit-workspace) .benefit-site-stars',
      'body:has(.benefit-workspace) .benefit-site-orbit',
      'body:has(.benefit-workspace) .benefit-site-planet',
    ]) {
      expect(
        declarations('src/styles/cinematic-site.css', selector).display
      ).toBe('none')
    }

    expect(
      declarations(
        'src/styles/cinematic-site.css',
        "body.benefit-cinematic-site [data-slot='card']"
      )['--tw-ring-shadow']
    ).toBe('0 0 #0000')

    expect(
      declarations(
        'src/styles/cinematic-site.css',
        "body.benefit-cinematic-site [data-slot='table'] td"
      )['font-variant-numeric']
    ).toBe('tabular-nums')
  })

  it('keeps 3d button hover lift and shadow intact while removing popup blur lag', () => {
    const indexCss = readFileSync(resolve('src/styles/index.css'), 'utf8')
    expect(indexCss).toContain('translate: 0 -2px 10px;')
    const popupKeyframe = indexCss.match(
      /@keyframes benefit-surface-3d-in\s*\{[\s\S]*?\n\}/
    )?.[0]
    expect(popupKeyframe).toBeDefined()
    expect(popupKeyframe).not.toContain('blur(')
  })

  it('aligns meta theme-color with dark forest and mist light palette', () => {
    const html = readFileSync(resolve('index.html'), 'utf8')
    expect(html).toContain('content="#081815"')
    expect(html).toContain("theme === 'dark' ? '#081815' : '#edf3ee'")
  })

  it('exposes language and theme controls on AuthLayout', async () => {
    const i18n = createInstance()
    await i18n.init({
      lng: 'en',
      fallbackLng: 'en',
      keySeparator: false,
      resources: { en },
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    queryClient.setQueryData(['status'], {
      system_name: 'Benefit API',
      logo: '/logo.png',
    })
    const rootRoute = createRootRoute({
      component: () => (
        <AuthLayout>
          <div>Sign in form</div>
        </AuthLayout>
      ),
    })
    const router = createRouter({
      routeTree: rootRoute,
      history: createMemoryHistory({ initialEntries: ['/sign-in'] }),
    })

    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </I18nextProvider>
    )

    expect(await screen.findByText('Sign in form')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Change language' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Toggle theme' })
    ).toBeInTheDocument()
  })
})
