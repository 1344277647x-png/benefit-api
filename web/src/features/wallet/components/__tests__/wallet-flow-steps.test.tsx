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
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { WalletFlowSteps } from '../wallet-flow-steps'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('WalletFlowSteps', () => {
  it('marks the selected payment stage as current and earlier stages complete', () => {
    render(<WalletFlowSteps currentStep={2} />)

    expect(screen.getByText('Payment Method').closest('li')).toHaveAttribute(
      'aria-current',
      'step'
    )
    expect(screen.getByText('Balance').closest('li')).toHaveAttribute(
      'data-state',
      'complete'
    )
    expect(screen.getByText('Confirm').closest('li')).toHaveAttribute(
      'data-state',
      'upcoming'
    )
  })

  it('clamps an out-of-range stage to confirmation', () => {
    render(<WalletFlowSteps currentStep={99} />)

    expect(screen.getByText('Confirm').closest('li')).toHaveAttribute(
      'aria-current',
      'step'
    )
  })
})
