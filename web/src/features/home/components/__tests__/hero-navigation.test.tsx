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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { Hero } from '../sections/hero'

vi.mock('three', async (importOriginal) => ({
  ...(await importOriginal<typeof import('three')>()),
  WebGLRenderer: class {
    constructor() {
      throw new Error('WebGL unavailable on this device')
    }
  },
}))

const clients: QueryClient[] = []

beforeEach(() => {
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...original(query),
    matches: query === '(hover: hover) and (pointer: fine)',
  }))
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(private callback: IntersectionObserverCallback) {}
      observe() {
        this.callback(
          [{ isIntersecting: true } as IntersectionObserverEntry],
          this as unknown as IntersectionObserver
        )
      }
      disconnect() {}
    }
  )
})

async function showHero(authenticated = false, docs = '') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(['status'], { docs_link: docs })
  const route = createRootRoute({
    component: () => <Hero isAuthenticated={authenticated} />,
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return screen.findByRole('heading', { level: 1 })
}

afterEach(async () => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  localStorage.clear()
  vi.unstubAllGlobals()
  await i18next.changeLanguage('en')
})

it('keeps signup and pricing keyboard-accessible when WebGL is unavailable', async () => {
  await showHero()
  const signup = screen.getByRole('link', { name: 'Get Started' })
  expect(signup).toHaveAttribute('href', '/sign-up')
  expect(screen.getByRole('link', { name: 'View Pricing' })).toHaveAttribute(
    'href',
    '/pricing'
  )
  const user = userEvent.setup()
  await user.tab()
  expect(signup).toHaveFocus()
  await user.tab()
  expect(screen.getByRole('link', { name: 'View Pricing' })).toHaveFocus()
  const canvas = document.querySelector('canvas')
  expect(canvas).toHaveAttribute('data-render-state', 'fallback')
  expect(canvas?.closest('[aria-hidden="true"]')).not.toBeNull()
})

it('shows dashboard and secure external docs for an authenticated visitor', async () => {
  await showHero(true, 'https://docs.example.test/guide')
  expect(screen.getByRole('link', { name: 'Go to Dashboard' })).toHaveAttribute(
    'href',
    '/dashboard'
  )
  const docs = screen.getByRole('link', { name: 'Docs' })
  expect(docs).toHaveAttribute('href', 'https://docs.example.test/guide')
  expect(docs).toHaveAttribute('rel', 'noopener noreferrer')
  expect(
    screen.queryByRole('link', { name: 'Get Started' })
  ).not.toBeInTheDocument()
})

it('retains the multi-model composition without WebGL and keeps decoration out of keyboard navigation', async () => {
  await showHero()
  for (const model of ['OpenAI', 'Claude', 'Gemini']) {
    const label = screen.getByText(model)
    expect(label).toBeVisible()
    expect(label.closest('[aria-hidden="true"]')).not.toBeNull()
  }
  const visual = screen.getByText('OpenAI').closest('.benefit-cinema-visual')
  expect(visual?.querySelector('.benefit-cinema-star-map')).toBeInTheDocument()
  expect(visual?.querySelector('.benefit-cinema-halo')).toBeInTheDocument()
  expect(visual?.querySelector('a, button, [tabindex="0"]')).toBeNull()
  expect(visual?.querySelector('canvas')).toHaveAttribute(
    'data-render-state',
    'fallback'
  )
})

it('retains configured internal documentation navigation', async () => {
  await showHero(true, '/docs')
  const docs = screen.getByRole('link', { name: 'Docs' })
  expect(docs).toHaveAttribute('href', '/docs')
  expect(docs).not.toHaveAttribute('target', '_blank')
})

it('tilts and highlights a model card on mouse movement and resets it when the pointer leaves', async () => {
  await showHero()
  const card = screen
    .getByText('OpenAI')
    .closest<HTMLElement>('.benefit-cinema-node')
  if (!card) throw new Error('Missing model card')
  vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: 100,
    height: 80,
  } as DOMRect)
  fireEvent(
    card,
    Object.assign(
      new MouseEvent('pointermove', {
        bubbles: true,
        clientX: 75,
        clientY: 20,
      }),
      { pointerType: 'mouse' }
    )
  )
  expect(card).toHaveAttribute('data-hovered', 'true')
  expect(card.style.getPropertyValue('--node-tilt-x')).toBe('3.5deg')
  expect(card.style.getPropertyValue('--node-tilt-y')).toBe('4.5deg')
  expect(card.style.getPropertyValue('--node-light-x')).toBe('75%')
  fireEvent.pointerOut(card)
  expect(card).not.toHaveAttribute('data-hovered')
})

it('does not tilt model cards for touch input or while decorative motion is paused', async () => {
  await showHero()
  const card = screen
    .getByText('OpenAI')
    .closest<HTMLElement>('.benefit-cinema-node')
  if (!card) throw new Error('Missing model card')
  vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: 100,
    height: 80,
  } as DOMRect)
  fireEvent(
    card,
    Object.assign(new MouseEvent('pointermove', { bubbles: true }), {
      pointerType: 'touch',
    })
  )
  expect(card).not.toHaveAttribute('data-hovered')
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
  fireEvent(document, new Event('visibilitychange'))
  fireEvent(
    card,
    Object.assign(new MouseEvent('pointermove', { bubbles: true }), {
      pointerType: 'mouse',
    })
  )
  expect(card).not.toHaveAttribute('data-hovered')
})

it('places the cosmic depth layer across the hero instead of confining it to the right visual', async () => {
  await showHero()
  const hero = screen.getByRole('region', { name: 'Benefit API' })
  const backdrop = hero.querySelector('.benefit-cinema-cosmos')
  expect(backdrop?.parentElement).toBe(hero)
  expect(backdrop).toHaveAttribute('aria-hidden', 'true')
  expect(
    backdrop?.querySelector('.benefit-cinema-galaxy-distant')
  ).toBeInTheDocument()
  expect(
    backdrop?.querySelector('.benefit-cinema-galaxy-near')
  ).toBeInTheDocument()
  expect(backdrop?.querySelector('a, button, [tabindex="0"]')).toBeNull()
})

it('renders the same signup action using the existing Chinese translations', async () => {
  i18next.addResourceBundle('en', 'translation', en.translation, true, true)
  i18next.addResourceBundle('zh', 'translation', zh.translation, true, true)
  await i18next.changeLanguage('zh')
  await showHero()
  expect(
    screen.getByRole('link', { name: zh.translation['Get Started'] })
  ).toHaveAttribute('href', '/sign-up')
  expect(
    screen.getByText(zh.translation['More models. Lower access cost.'])
  ).toBeInTheDocument()
})
