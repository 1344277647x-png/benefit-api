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
