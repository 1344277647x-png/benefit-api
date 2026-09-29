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

import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { PendingTeamPayment } from '../pending-team-payment'

describe('pending team payment recovery', () => {
  test('owner sees the original order and can reopen it when unpaid', () => {
    const onResume = vi.fn()
    const onCancel = vi.fn()
    render(
      <PendingTeamPayment
        payment={{
          plan_title: 'Shared plan',
          money: 10.5,
          payment_method: 'alipay',
          create_time: 100,
        }}
        busy={false}
        onResume={onResume}
        onCancel={onCancel}
      />
    )

    expect(screen.getByText(/Shared plan.*10\.50.*alipay/)).toBeInTheDocument()
    expect(
      screen.getByText(/If you already paid, do not pay again/)
    ).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Resume original payment' })
    )
    expect(onResume).toHaveBeenCalledOnce()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel unpaid order' }))
    expect(onCancel).toHaveBeenCalledOnce()
  })

  test('reopening is disabled while another payment request is in progress', () => {
    render(
      <PendingTeamPayment
        payment={{
          plan_title: 'Shared plan',
          money: 10,
          payment_method: 'alipay',
          create_time: 100,
        }}
        busy
        onResume={vi.fn()}
        onCancel={vi.fn()}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Resume original payment' })
    ).toBeDisabled()
  })
})
