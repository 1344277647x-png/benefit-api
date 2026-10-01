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
import { ERROR_MESSAGES } from '../../constants'

type RequestErrorLike = {
  message?: string
  response?: {
    data?: {
      error?: {
        code?: string
        message?: string
      }
      message?: string
    }
  }
}

const UPSTREAM_QUOTA_EXHAUSTED = 'upstream_quota_exhausted'
const LEGACY_UPSTREAM_QUOTA_CODES = new Set([
  'insufficient_quota',
  'quota_exceeded',
])
const PLATFORM_QUOTA_CODES = new Set([
  'insufficient_user_quota',
  'pre_consume_token_quota_failed',
])

function isUpstreamQuotaError(
  errorCode: string | undefined,
  message: string
): boolean {
  if (
    errorCode === UPSTREAM_QUOTA_EXHAUSTED ||
    LEGACY_UPSTREAM_QUOTA_CODES.has(errorCode || '')
  ) {
    return true
  }
  if (PLATFORM_QUOTA_CODES.has(errorCode || '')) return false
  const normalized = message.toLowerCase()
  return [
    'insufficient_quota',
    'quota exceeded',
    'quota_exceeded',
    'exceeded your current quota',
    'billing_hard_limit_reached',
    'credits exhausted',
    'insufficient credits',
    'billing limit',
    'account balance insufficient',
  ].some((signal) => normalized.includes(signal))
}

function safeErrorMessage(
  errorCode: string | undefined,
  message: string
): string {
  if (isUpstreamQuotaError(errorCode, message)) {
    return ERROR_MESSAGES.UPSTREAM_QUOTA_EXHAUSTED
  }
  return message
}

export type RequestErrorDetails = {
  errorCode?: string
  errorMessage: string
}

export function parseRequestErrorDetails(error: unknown): RequestErrorDetails {
  const requestError = error as RequestErrorLike
  const errorCode = requestError?.response?.data?.error?.code || undefined
  const message =
    requestError?.response?.data?.error?.message ||
    requestError?.response?.data?.message ||
    requestError?.message ||
    ERROR_MESSAGES.API_REQUEST_ERROR

  return {
    errorCode,
    errorMessage: safeErrorMessage(errorCode, message),
  }
}
