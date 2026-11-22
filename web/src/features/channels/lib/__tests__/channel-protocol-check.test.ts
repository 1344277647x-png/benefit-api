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
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { handleTestChannel } from '../channel-actions'

afterEach(() => vi.restoreAllMocks())

it('forwards protocol_check param and returns structured protocol check report', async () => {
  const protocolReport = {
    mode: 'responses_stream' as const,
    passed: true,
    events_seen: ['response.created', 'response.completed'],
    checks: [
      {
        name: 'response_completed',
        passed: true,
        detail: 'Observed response.completed',
      },
    ],
    summary: 'Responses stream lifecycle check passed',
  }
  const getSpy = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      time: 0.42,
      protocol_check: protocolReport,
    },
  })

  const onComplete = vi.fn()
  await handleTestChannel(
    111,
    {
      channelName: 'Codex-111',
      testModel: 'gpt-5-codex',
      endpointType: 'openai-response',
      stream: true,
      group: 'default',
      protocolCheck: 'responses_stream',
      silent: true,
    },
    onComplete
  )

  expect(getSpy).toHaveBeenCalledWith(
    '/api/channel/test/111',
    expect.objectContaining({
      params: {
        model: 'gpt-5-codex',
        endpoint_type: 'openai-response',
        stream: true,
        group: 'default',
        protocol_check: 'responses_stream',
      },
    })
  )
  expect(onComplete).toHaveBeenCalledWith(
    true,
    420,
    undefined,
    undefined,
    protocolReport
  )
})
