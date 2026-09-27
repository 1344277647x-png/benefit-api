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

import { SectionPageLayout } from '../components/section-page-layout'

describe('Section layout scrolling', () => {
  it('keeps the title and actions outside the scrollable content region', () => {
    const view = render(
      <SectionPageLayout>
        <SectionPageLayout.Title>
          Channels with a long title
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <button type='button'>Manage channels</button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <p>Bottom of channel health</p>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
    const content = view.container.querySelector('[data-slot="page-content"]')
    expect(content).toHaveClass('min-h-0', 'overflow-auto')
    expect(content).toContainElement(
      screen.getByText('Bottom of channel health')
    )
    expect(content).not.toContainElement(screen.getByRole('button'))
    expect(content).not.toContainElement(screen.getByRole('heading'))
  })

  it('lets fixed data tables own scrolling without adding a second scroll area', () => {
    const view = render(
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Content>Table</SectionPageLayout.Content>
      </SectionPageLayout>
    )
    expect(
      view.container.querySelector('[data-slot="page-content"]')
    ).toHaveClass('overflow-hidden')
  })
})
