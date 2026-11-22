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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { TeamPage } from '../../index'

type ApiMethod = (
  url: string,
  data?: unknown
) => Promise<{ data: Record<string, unknown> }>

const apiClient = api as unknown as { get: ApiMethod; post: ApiMethod }
const originalGet = apiClient.get
const originalPost = apiClient.post
const i18n = createInstance()

await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

afterEach(() => {
  apiClient.get = originalGet
  apiClient.post = originalPost
  useAuthStore.getState().auth.reset('idle')
})

describe('team token settings', () => {
  test('selects a group and its allowed models and shows restrictions in the token list', async () => {
    const user = userEvent.setup()
    const createdPayloads: Array<Record<string, unknown>> = []
    apiClient.get = async (url, config) => {
      if (url === '/api/team/self') {
        return {
          data: {
            success: true,
            data: {
              team: { id: 7, name: 'Demo', status: 'active', owner_id: 1 },
              membership: { user_id: 2, role: 'member' },
              subscription: {
                id: 3,
                plan_title: 'Team',
                end_time: 2_000_000_000,
                seat_limit: 3,
              },
              period: {
                amount_total: 1000,
                amount_used: 0,
                end_time: 2_000_000_000,
              },
              invitations: [],
            },
          },
        }
      }
      if (url === '/api/team/tokens') {
        return {
          data: {
            success: true,
            data: [
              {
                id: 8,
                name: 'Existing key',
                enabled: true,
                created_time: 1,
                group: 'CodexPro',
                group_inherited: false,
                model_limits_enabled: true,
                model_limits: ['gpt-5-pro'],
              },
            ],
          },
        }
      }
      if (url === '/api/team/usage') {
        return { data: { success: true, data: { amount: 0, requests: 0 } } }
      }
      if (url === '/api/user/self/groups') {
        return {
          data: {
            success: true,
            data: {
              auto: { desc: 'Automatic', ratio: 'auto' },
              CodexPlus: { desc: 'Plus', ratio: 1 },
              CodexPro: { desc: 'Pro', ratio: 2 },
            },
          },
        }
      }
      if (url === '/api/user/models') {
        const group = (config as { params?: { group?: string } })?.params?.group
        return {
          data: {
            success: true,
            data: group === 'CodexPro' ? ['gpt-5-pro'] : ['gpt-5-codex'],
          },
        }
      }
      throw new Error(`Unexpected GET ${url}`)
    }
    apiClient.post = async (url, body) => {
      expect(url).toBe('/api/team/tokens')
      createdPayloads.push(body as Record<string, unknown>)
      return {
        data: {
          success: true,
          data: { id: 9, name: 'Restricted key', key: 'sk-test' },
        },
      }
    }
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'member',
      role: 1,
      group: 'CodexPlus',
    })
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <TeamPage />
        </I18nextProvider>
      </QueryClientProvider>
    )

    expect(await screen.findByText(/Existing key/)).toBeTruthy()
    expect(screen.getByText(/CodexPro.*gpt-5-pro/)).toBeTruthy()
    const groupSelect = await screen.findByLabelText('Group')
    expect(groupSelect.querySelector('option[value="auto"]')).toBeNull()
    await user.selectOptions(groupSelect, 'CodexPro')

    const modelInput = screen.getByLabelText(
      'Select models (empty for allow all)'
    )
    await waitFor(() => expect(modelInput).toBeEnabled())
    await user.click(modelInput)
    await user.type(modelInput, 'gpt-5-pro')
    const modelOption = await waitFor(() => {
      const option = document.querySelector<HTMLElement>(
        '[data-slot="combobox-item"]'
      )
      expect(option?.textContent).toContain('gpt-5-pro')
      return option as HTMLElement
    })
    await user.click(modelOption)
    fireEvent.input(screen.getByLabelText('Key name'), {
      target: { value: 'Restricted key' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create team key' }))

    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]).toMatchObject({
      name: 'Restricted key',
      group: 'CodexPro',
      model_limits: 'gpt-5-pro',
    })
    queryClient.clear()
  })
})
