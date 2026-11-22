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
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { usePaymentConfirmation } from '../use-payment-confirmation'

beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

it('checks only the returned order, even if another payment settled first', async () => {
  const get = vi.spyOn(api, 'get')
  const response = (id: string) => ({
    data: {
      success: true,
      data: { items: [{ trade_no: id, status: 'success' }] },
    },
  })
  get
    .mockResolvedValueOnce(response('order-other'))
    .mockResolvedValue(response('order-expected'))
  const onConfirmed = vi.fn()
  renderHook(() =>
    usePaymentConfirmation({ tradeNo: 'order-expected', onConfirmed })
  )
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(onConfirmed).not.toHaveBeenCalled()
  await act(() => vi.advanceTimersByTimeAsync(2000))
  expect(onConfirmed).toHaveBeenCalledTimes(1)
  expect(get).toHaveBeenCalledWith(
    expect.stringContaining('/topup/self?'),
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
  expect(get.mock.calls[0][0]).toContain('keyword=order-expected')
})

it('stops after bounded failed checks, supports retry, and aborts on unmount', async () => {
  const get = vi.spyOn(api, 'get').mockRejectedValue(new Error('offline'))
  const onConfirmed = vi.fn()
  const { result, unmount } = renderHook(() =>
    usePaymentConfirmation({ tradeNo: 'order', onConfirmed })
  )
  await act(() => vi.runAllTimersAsync())
  expect(result.current.state).toBe('timeout')
  expect(get).toHaveBeenCalledTimes(5)
  act(() => result.current.retry())
  expect(result.current.state).toBe('checking')
  unmount()
  expect(get.mock.lastCall?.[1]?.signal?.aborted).toBe(true)
  await act(() => vi.runAllTimersAsync())
  expect(get).toHaveBeenCalledTimes(6)
  expect(onConfirmed).not.toHaveBeenCalled()
})

it('does not guess a payment when the return has no order reference', () => {
  const get = vi.spyOn(api, 'get')
  const { result } = renderHook(() =>
    usePaymentConfirmation({ onConfirmed: vi.fn() })
  )
  expect(result.current.state).toBe('idle')
  expect(get).not.toHaveBeenCalled()
})
