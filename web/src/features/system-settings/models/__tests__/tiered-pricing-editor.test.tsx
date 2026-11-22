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
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, it, vi } from 'vitest'

import { TieredPricingEditor } from '../tiered-pricing-editor'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    i18n: { language: 'en' },
    t: (key: string, values?: Record<string, string | number>) =>
      key.replaceAll(/\{\{(\w+)\}\}/g, (match, name: string) =>
        String(values?.[name] ?? match)
      ),
  }),
}))

function Editor() {
  const [expr, setExpr] = useState(
    'len <= 32000 ? tier("short", p * 1 + c * 2) : tier("long", p * 2 + c * 4)'
  )
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

it('keeps the price input mounted while typing and when a preceding tier is removed', async () => {
  render(<Editor />)
  const user = userEvent.setup()
  await user.click(
    screen.getByRole('button', { name: 'Edit pricing rule long' })
  )
  const tier = within(screen.getByRole('group', { name: 'Pricing tier long' }))
  const price = tier.getByRole('textbox', { name: 'Input price' })
  await user.click(price)
  for (const value of ['1', '1.2', '1.23']) {
    fireEvent.change(price, { target: { value } })
    expect(price).toHaveFocus()
    expect(price).toHaveValue(value)
  }
  await user.click(screen.getByRole('button', { name: 'Branch actions 1' }))
  await user.click(screen.getByRole('menuitem', { name: 'Remove branch' }))
  expect(screen.getByRole('textbox', { name: 'Input price' })).toBe(price)
  expect(price).toHaveValue('1.23')
})
