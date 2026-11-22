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
import { Activity } from 'lucide-react'
import { describe, expect, it } from 'vitest'

import { StatCard } from '../stat-card'

describe('Dashboard metric presentation', () => {
  it('keeps long values readable and contained in the metric column', () => {
    const view = render(
      <StatCard
        title='Requests'
        value='1,234,567,890'
        description='Total requests'
        icon={Activity}
      />
    )
    expect(view.container.firstElementChild).toHaveClass('min-w-0')
    expect(screen.getByText('1,234,567,890')).toHaveClass(
      'break-all',
      'tabular-nums'
    )
    expect(screen.getByText('Total requests')).toBeVisible()
  })

  it('does not show a stale number when the metric request fails', () => {
    render(
      <StatCard
        title='Requests'
        value='123'
        description='Unavailable'
        icon={Activity}
        error
      />
    )
    expect(screen.queryByText('123')).not.toBeInTheDocument()
    expect(screen.getByText('--')).toBeVisible()
    expect(screen.getByText('Unavailable')).toBeVisible()
  })
})
