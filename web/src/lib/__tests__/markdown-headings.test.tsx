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
import { describe, expect, it, vi } from 'vitest'

import { Markdown } from '@/components/ui/markdown'
import { ApiEndpointBar } from '@/features/keys/components/api-endpoint-bar'
import { WalletStatsCard } from '@/features/wallet/components/wallet-stats-card'

describe('markdown-headings', () => {
  it('reports the rendered headings and uses unique, working anchors', () => {
    const markdown = [
      '# Title (ignored)',
      '## Quick Start',
      '### 接入配置 &amp; **Cursor**',
      '```bash',
      '## Fake Heading Inside Code',
      '```',
      '## Quick Start',
      '## Quick Start 2',
      'Setext heading',
      '--------------',
    ].join('\n')
    const onHeadingsChange = vi.fn()
    render(<Markdown onHeadingsChange={onHeadingsChange}>{markdown}</Markdown>)
    expect(onHeadingsChange).toHaveBeenLastCalledWith([
      { id: 'docs-quick-start', level: 2, text: 'Quick Start' },
      { id: 'docs-接入配置-cursor', level: 3, text: '接入配置 & Cursor' },
      { id: 'docs-quick-start-2', level: 2, text: 'Quick Start' },
      { id: 'docs-quick-start-2-2', level: 2, text: 'Quick Start 2' },
      { id: 'docs-setext-heading', level: 2, text: 'Setext heading' },
    ])
    expect(
      screen.getByRole('heading', { name: 'Quick Start 2' })
    ).toHaveAttribute('id', 'docs-quick-start-2-2')
  })
})

describe('ApiEndpointBar and WalletStatsCard', () => {
  it('renders API base URL quick bar with copy button and docs link', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const route = createRootRoute({
      component: () => <ApiEndpointBar />,
    })
    const router = createRouter({
      routeTree: route,
      history: createMemoryHistory({ initialEntries: ['/keys'] }),
    })

    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    expect(await screen.findByText('API base URL')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy URL' })).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Client setup guide/i })
    ).toHaveAttribute('href', '/docs')
  })

  it('renders WalletStatsCard mobile-first grid with full-width balance cell', () => {
    const { container } = render(
      <WalletStatsCard
        user={{
          id: 1,
          username: 'tester',
          quota: 2500000,
          used_quota: 500000,
          request_count: 42,
          aff_quota: 0,
          aff_history_quota: 0,
          aff_count: 0,
          group: 'default',
        }}
        loading={false}
      />
    )
    expect(screen.getByText('Current Balance')).toBeInTheDocument()
    expect(screen.getByText('Total Usage')).toBeInTheDocument()
    expect(screen.getByText('API Requests')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(
      container.querySelector('.grid-cols-2.sm\\:grid-cols-3')
    ).not.toBeNull()
  })
})
