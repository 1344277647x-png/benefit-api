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
import { describe, expect, it } from 'vitest'

import { Main } from '../components/main'

describe('Main layout containment', () => {
  it('allows wide content to shrink within the sidebar instead of widening the viewport', () => {
    render(
      <Main>
        <div style={{ minWidth: 1400 }}>Long operational table</div>
      </Main>
    )
    expect(screen.getByRole('main')).toHaveClass(
      'min-w-0',
      'min-h-0',
      'overflow-hidden'
    )
    expect(screen.getByText('Long operational table')).toBeInTheDocument()
  })

  it('preserves page-level vertical scrolling when a page opts in', () => {
    render(<Main className='overflow-auto'>Page footer</Main>)
    expect(screen.getByRole('main')).toHaveClass('overflow-auto', 'min-w-0')
    expect(screen.getByRole('main')).not.toHaveClass('overflow-hidden')
  })
})
