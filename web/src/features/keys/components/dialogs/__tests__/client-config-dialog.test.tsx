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
import { afterEach, expect, it, vi } from 'vitest'

import * as clientApi from '@/lib/api'
import * as clipboard from '@/lib/copy-to-clipboard'

import { ClientConfigDialog } from '../client-config-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (s: string) => s }),
}))
afterEach(() => vi.restoreAllMocks())

it.each(['Codex CLI', 'Claude Code', 'Cherry Studio', 'Cursor'])(
  'keeps %s preview private and copies the selected key on demand',
  async (label) => {
    vi.spyOn(clientApi, 'getUserModels').mockResolvedValue({
      success: true,
      data: ['fixture-model'],
    })
    const copy = vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(true)
    const resolveKey = vi.fn().mockResolvedValue('fixture-secret')
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ClientConfigDialog
          open
          onOpenChange={() => {}}
          resolveKey={resolveKey}
        />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('radio', { name: label }))
    const input = screen.getByPlaceholderText('Select or enter model name')
    fireEvent.change(input, { target: { value: 'fixture-model' } })
    expect(screen.getByLabelText('Configuration preview')).toHaveTextContent(
      'YOUR_API_KEY'
    )
    expect(resolveKey).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Copy configuration' }))
    await waitFor(() => expect(copy).toHaveBeenCalledTimes(1))
    const copied = copy.mock.calls[0][0]
    expect(copied).toContain('sk-fixture-secret')
    expect(copied).toContain('fixture-model')
    expect(copied).not.toContain('YOUR_API_KEY')
    expect(
      screen.getByLabelText('Configuration preview')
    ).not.toHaveTextContent('fixture-secret')
  }
)
