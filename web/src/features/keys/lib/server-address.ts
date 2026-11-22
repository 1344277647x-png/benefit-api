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
/**
 * Public server address configured by the administrator (cached `status`
 * from /api/status), falling back to the current origin.
 */
export function getServerAddress(): string {
  try {
    const raw = localStorage.getItem('status')
    if (raw) {
      const status = JSON.parse(raw)
      if (
        typeof status.server_address === 'string' &&
        status.server_address.trim()
      ) {
        return status.server_address.trim().replace(/\/+$/, '')
      }
    }
  } catch {
    /* empty */
  }
  return window.location.origin
}

/** OpenAI-compatible base URL (`<server>/v1`) used by client templates. */
export function getApiBaseUrl(): string {
  const address = getServerAddress()
  return address.endsWith('/v1') ? address : `${address}/v1`
}

/** Normalizes a token to the `sk-` form expected by clients. */
export function toClientApiKey(tokenKey: string): string {
  return tokenKey.startsWith('sk-') ? tokenKey : `sk-${tokenKey}`
}
