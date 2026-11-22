/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { ERROR_MESSAGES } from '../../constants'
import { parseRequestErrorDetails } from './request-error-utils'

describe('parseRequestErrorDetails', () => {
  test('maps upstream quota errors to the safe public message', () => {
    const result = parseRequestErrorDetails({
      response: {
        data: {
          error: {
            code: 'upstream_quota_exhausted',
            message: 'insufficient_quota: account balance insufficient',
          },
        },
      },
    })

    expect(result.errorCode).toBe('upstream_quota_exhausted')
    expect(result.errorMessage).toBe(ERROR_MESSAGES.UPSTREAM_QUOTA_EXHAUSTED)
    expect(result.errorMessage).not.toContain('insufficient_quota')
  })

  test('supports nested error messages without changing other errors', () => {
    expect(
      parseRequestErrorDetails({
        response: {
          data: {
            error: { code: 'rate_limit_exceeded', message: 'try later' },
          },
        },
      }).errorMessage
    ).toBe('try later')
  })

  test('preserves platform quota errors even when their text resembles upstream quota', () => {
    const message = 'Your account balance is insufficient.'
    const result = parseRequestErrorDetails({
      response: {
        data: {
          error: { code: 'insufficient_user_quota', message },
        },
      },
    })

    expect(result.errorMessage).toBe(message)
  })
})
