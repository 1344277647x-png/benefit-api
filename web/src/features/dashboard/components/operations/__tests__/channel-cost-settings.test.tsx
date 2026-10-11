import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ChannelCostSettings } from '../channel-cost-settings'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function renderSettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelCostSettings />
    </QueryClientProvider>
  )
  return client
}

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
})
afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('Channel cost version settings', () => {
  it('shows unknown cost when a channel has no versions and saves an explicit multiplier', async () => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true } })
    renderSettings()
    fireEvent.change(screen.getByLabelText('Channel ID'), {
      target: { value: '8' },
    })
    expect(
      await screen.findByText(
        'No cost version configured; cost remains unknown.'
      )
    ).toBeVisible()
    fireEvent.change(screen.getByLabelText('Upstream multiplier'), {
      target: { value: '0.125' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save cost version' }))
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/log/operations-cost-versions', {
        channel_id: 8,
        multiplier: '0.125',
        effective_at: 0,
      })
    )
  })

  it('allows an ordinary administrator to read but disables editing and saving', async () => {
    useAuthStore.getState().auth.setUser({ id: 2, username: 'admin', role: 10 })
    const post = vi.spyOn(api, 'post')
    renderSettings()
    fireEvent.change(screen.getByLabelText('Channel ID'), {
      target: { value: '8' },
    })
    expect(
      await screen.findByText(
        'No cost version configured; cost remains unknown.'
      )
    ).toBeVisible()
    expect(screen.getByLabelText('Upstream multiplier')).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Save cost version' })
    ).toBeDisabled()
    expect(post).not.toHaveBeenCalled()
  })

  it('rejects negative multipliers and distinguishes failed loading from an empty history', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('unavailable'))
    const post = vi.spyOn(api, 'post')
    renderSettings()
    fireEvent.change(screen.getByLabelText('Channel ID'), {
      target: { value: '8' },
    })
    expect(await screen.findByRole('alert')).toHaveTextContent('Request failed')
    expect(
      screen.queryByText('No cost version configured; cost remains unknown.')
    ).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Upstream multiplier'), {
      target: { value: '-1' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save cost version' }))
    expect(post).not.toHaveBeenCalled()
  })
})
