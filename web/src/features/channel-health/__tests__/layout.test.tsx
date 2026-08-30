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
import { render, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { ChannelHealthItems } from '../admin-panel'
import type { ChannelHealthView } from '../api'

const item: ChannelHealthView = {
  channel_id: 18,
  model: 'gpt-5.6-sol-with-a-very-long-model-name',
  endpoint_type: 'responses',
  status: 'delayed',
  request_count: 12,
  success_rate: 91.7,
  average_latency_ms: 12600,
  average_ttft_ms: 10800,
  consecutive_failures: 0,
  last_sample_at: 1,
  last_success_at: 1,
  last_error_code: 'upstream_timeout',
}

describe('ChannelHealthItems layout', () => {
  it('uses a vertical bounded list on mobile without the wide desktop grid', () => {
    const view = render(<ChannelHealthItems items={[item]} t={(key) => key} />)
    const mobile = view.container.querySelector(
      '[data-channel-health-layout="mobile"]'
    )

    expect(mobile).toHaveClass('sm:hidden', 'overflow-y-auto')
    expect(mobile).not.toHaveClass('overflow-x-auto')
    expect(
      within(mobile as HTMLElement).getByRole('listitem')
    ).toHaveTextContent(item.model)
  })

  it('keeps the desktop grid horizontally scrollable with a stable minimum width', () => {
    const view = render(<ChannelHealthItems items={[item]} t={(key) => key} />)
    const desktop = view.container.querySelector(
      '[data-channel-health-layout="desktop"]'
    )

    expect(desktop).toHaveClass('overflow-x-auto', 'sm:block')
    expect(desktop?.firstElementChild).toHaveClass('min-w-[760px]')
  })
})
