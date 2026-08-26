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
import { describe, expect, test } from 'vitest'

import { getRetryAfterSeconds } from '../http-client'

describe('rate-limit feedback', () => {
  test('extracts the remaining wait from a 429 Retry-After header', () => {
    expect(
      getRetryAfterSeconds({
        response: { status: 429, headers: { 'retry-after': '42' } },
      })
    ).toBe(42)
  })

  test('rejects malformed waits and responses that are not rate-limited', () => {
    expect(
      getRetryAfterSeconds({
        response: { status: 429, headers: { 'retry-after': '42 seconds' } },
      })
    ).toBe(null)
    expect(
      getRetryAfterSeconds({
        response: { status: 503, headers: { 'retry-after': '42' } },
      })
    ).toBe(null)
  })
})
