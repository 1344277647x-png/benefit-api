import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UsersProvider } from '../users-provider'
import { UsersTable } from '../users-table'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const search = { filter: '349' }
  const navigate = () => undefined
  return {
    ...(await importOriginal<typeof import('@tanstack/react-router')>()),
    getRouteApi: () => ({
      useSearch: () => search,
      useNavigate: () => navigate,
    }),
  }
})

test('exact ID search shows the account before usage and retains it when usage fails', async () => {
  const user = {
    id: 349,
    username: 'lookup-account',
    display_name: 'Lookup account',
    role: 1,
    status: 1,
    quota: 500,
    used_quota: 0,
    request_count: 0,
    group: 'default',
  }
  let finishUsage!: () => void
  const usage = new Promise<void>((resolve) => {
    finishUsage = resolve
  })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url: string) => {
    if (url === '/api/user/usage-summary') {
      await usage
      return { data: { success: false } }
    }
    const query = new URL(url, 'http://localhost').searchParams
    if (query.get('include_usage') !== 'false') {
      throw new Error('Account request must not wait for usage')
    }
    return { data: { success: true, data: { items: [user], total: 1 } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  try {
    render(
      <QueryClientProvider client={client}>
        <UsersProvider>
          <UsersTable />
        </UsersProvider>
      </QueryClientProvider>
    )
    fireEvent.change(screen.getByRole('combobox', { name: 'Search mode' }), {
      target: { value: 'id' },
    })
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith(
        expect.stringMatching(/keyword=349&exact_field=id/)
      )
    )
    expect(await screen.findByText('lookup-account')).toBeVisible()
    expect(await screen.findByText('Loading usage summary')).toBeVisible()
    finishUsage()
    expect(
      await screen.findByText('Failed to load usage summary')
    ).toBeVisible()
    expect(screen.getByText('lookup-account')).toBeVisible()

    get.mockImplementationOnce(async () => ({
      data: {
        success: true,
        data: {
          349: { token_used: 432, request_count: 9, consumed_quota: 10 },
        },
      },
    }))
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('432')).toBeVisible()
    await waitFor(() =>
      expect(screen.queryByText('Failed to load usage summary')).toBeNull()
    )
  } finally {
    client.clear()
  }
})
