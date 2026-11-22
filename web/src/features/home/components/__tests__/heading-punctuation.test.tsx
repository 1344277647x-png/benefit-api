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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import viLocale from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import { Features } from '../sections/features'
import { Hero } from '../sections/hero'

vi.mock('three', async (importOriginal) => ({
  ...(await importOriginal<typeof import('three')>()),
  WebGLRenderer: class {
    constructor() {
      throw new Error('WebGL unavailable on this device')
    }
  },
}))

const resources = { en, fr, ja, ru, vi: viLocale, zh, 'zh-TW': zhTW }
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(['status'], { docs_link: '' })
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
})

afterEach(() => {
  client.clear()
  localStorage.clear()
  vi.unstubAllGlobals()
})

it.each(Object.keys(resources))(
  'renders %s marketing headlines without trailing full stops while preserving body punctuation',
  async (language) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: 'en',
      keySeparator: false,
      resources,
    })
    const route = createRootRoute({
      component: () => (
        <>
          <Hero isAuthenticated={false} />
          <Features />
        </>
      ),
    })
    const router = createRouter({
      routeTree: route,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={client}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </I18nextProvider>
    )
    await screen.findByRole('heading', { level: 1 })

    expect(
      screen.getByText(i18n.t('More models. Lower access cost.')).textContent
    ).not.toMatch(/[.。．]\s*$/u)
    for (const heading of screen.getAllByRole('heading')) {
      expect(heading.textContent).not.toMatch(/[.。．]\s*$/u)
    }
    expect(
      screen.getByRole('heading', {
        name: i18n.t('Spend less on access. Build more.'),
        level: 2,
      })
    ).toBeInTheDocument()
    for (const key of [
      'One OpenAI-compatible address connects leading models. Pay by actual usage, with balance and billing always visible.',
      'Recharge online, choose the model you need, and review every charge in the console.',
    ]) {
      expect(screen.getByText(i18n.t(key)).textContent).toMatch(/[.。．]\s*$/u)
    }
  }
)
