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
*/
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { usageLogSchema } from '../../../data/schema'
import { DetailsDialog } from '../details-dialog'

describe('team pricing snapshot in usage log details', () => {
  test('shows an explicit warning when a settled team log lacks its full pricing snapshot', async () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 2,
      created_at: 1,
      type: 2,
      content: '',
      other: JSON.stringify({
        billing_source: 'team',
        billing_mode: 'tiered_expr',
        matched_tier: 'long',
        pricing_snapshot_incomplete: true,
      }),
    })

    render(
      <DetailsDialog
        log={log}
        isAdmin={false}
        open
        onOpenChange={() => undefined}
      />
    )

    expect(await screen.findByRole('status')).toHaveTextContent(
      'Some historical pricing details are unavailable for this team request.'
    )
  })
})
