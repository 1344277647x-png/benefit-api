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
