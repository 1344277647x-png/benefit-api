/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { ERROR_MESSAGES } from '../../constants'
import { parseStreamErrorDetails } from './stream-utils'

describe('parseStreamErrorDetails', () => {
  test('does not expose an upstream quota message from SSE', () => {
    const result = parseStreamErrorDetails(
      JSON.stringify({
        error: {
          code: 'upstream_quota_exhausted',
          message: 'credits exhausted at upstream provider',
        },
      })
    )

    expect(result.errorMessage).toBe(ERROR_MESSAGES.UPSTREAM_QUOTA_EXHAUSTED)
    expect(result.errorMessage).not.toContain('credits exhausted')
  })

  test('preserves a platform insufficient-balance message from SSE', () => {
    const message = 'Your account balance is insufficient.'
    const result = parseStreamErrorDetails(
      JSON.stringify({
        error: { code: 'insufficient_user_quota', message },
      })
    )

    expect(result.errorMessage).toBe(message)
  })
})
