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
import { useCallback, useEffect, useRef, useState } from 'react'

import { getOwnOrderQuietly, isApiSuccess } from '../api'
import {
  isConfirmedOrder,
  PAYMENT_CONFIRMATION_DELAYS_MS,
} from '../lib/payment-confirmation'

export type PaymentConfirmationState = 'idle' | 'checking' | 'timeout'

export function usePaymentConfirmation(options: {
  tradeNo?: string
  onConfirmed: () => void
}) {
  const [retryCount, setRetryCount] = useState(0)
  const [exhaustedRun, setExhaustedRun] = useState('')
  const run = JSON.stringify([options.tradeNo, retryCount])
  const onConfirmed = useRef(options.onConfirmed)
  useEffect(() => {
    onConfirmed.current = options.onConfirmed
  }, [options.onConfirmed])

  useEffect(() => {
    const order = options.tradeNo
    if (!order) return
    let attempt = 0
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function check(tradeNo: string) {
      let confirmed = false
      try {
        const result = await getOwnOrderQuietly(tradeNo, controller.signal)
        confirmed =
          isApiSuccess(result) && isConfirmedOrder(tradeNo, result.data?.items)
      } catch {
        // A temporary network failure uses the same bounded retry schedule.
      }
      if (controller.signal.aborted) return
      if (confirmed) {
        onConfirmed.current()
        return
      }
      if (attempt === PAYMENT_CONFIRMATION_DELAYS_MS.length) {
        setExhaustedRun(run)
        return
      }
      timer = setTimeout(
        () => void check(tradeNo),
        PAYMENT_CONFIRMATION_DELAYS_MS[attempt++]
      )
    }
    void check(order)
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [options.tradeNo, run])

  let state: PaymentConfirmationState = 'idle'
  if (options.tradeNo) state = run === exhaustedRun ? 'timeout' : 'checking'
  const retry = useCallback(() => setRetryCount((count) => count + 1), [])
  return { state, retry }
}
