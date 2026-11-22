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
import { act, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { expect, it, vi } from 'vitest'

import { TieredPricingEditor } from '../tiered-pricing-editor'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

function Editor() {
  const [expr, setExpr] = useState('')
  const [rule, setRule] = useState('')
  return (
    <TieredPricingEditor
      billingExpr={expr}
      requestRuleExpr={rule}
      onBillingExprChange={setExpr}
      onRequestRuleExprChange={setRule}
    />
  )
}

it('keeps the price input mounted while typing and when a preceding tier is removed', () => {
  render(<Editor />)
  fireEvent.click(screen.getByRole('button', { name: 'Add tier' }))
  const price = screen.getAllByRole<HTMLInputElement>('spinbutton', {
    name: 'Input price',
  })[1]
  act(() => price.focus())
  for (const value of ['1', '1.2', '1.23']) {
    fireEvent.change(price, { target: { value } })
    expect(document.activeElement).toBe(price)
    expect(price.value).toBe(value)
  }
  fireEvent.click(screen.getAllByRole('button', { name: 'Remove tier' })[0])
  expect(screen.getByRole('spinbutton', { name: 'Input price' })).toBe(price)
  expect(price.value).toBe('1.23')
})
