/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

/**
 * Browser-only progress markers for the first-use guide.
 * No credentials or request payloads are persisted here.
 */
export const ONBOARDING_STORAGE_KEYS = {
  apiUrlCopied: 'benefit_api_onboarding_api_url_copied',
  configurationCopied: 'benefit_api_onboarding_configuration_copied',
  modelSelected: 'benefit_api_onboarding_model_selected',
  logsViewed: 'benefit_api_onboarding_logs_viewed',
} as const

export function readOnboardingFlag(key: string): boolean {
  if (typeof window === 'undefined') return false

  try {
    return window.localStorage.getItem(key) === '1'
  } catch {
    return false
  }
}

export function markOnboardingFlag(key: string): void {
  if (typeof window === 'undefined') return

  try {
    window.localStorage.setItem(key, '1')
  } catch {
    // Private browsing or disabled storage should not block core actions.
  }
}
