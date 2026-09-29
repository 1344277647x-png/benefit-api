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

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { PendingTeamPayment as PendingPayment } from '../types'

export function PendingTeamPayment(props: {
  payment: PendingPayment
  busy: boolean
  onResume: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()

  return (
    <section className='border-warning/50 bg-card/75 min-w-0 rounded-2xl border p-4 sm:p-6'>
      <h2 className='mb-2 text-lg font-semibold'>
        {t('Pending team payment')}
      </h2>
      <p className='break-words'>
        {props.payment.plan_title} · {props.payment.money.toFixed(2)} ·{' '}
        {props.payment.payment_method}
      </p>
      <p className='text-muted-foreground mt-2 text-sm'>
        {t(
          'A previous online order is awaiting confirmation. New purchases are paused to avoid duplicate charges.'
        )}
      </p>
      <p className='text-muted-foreground mt-1 text-sm'>
        {t(
          'If you already paid, do not pay again. Contact support with your payment receipt if the subscription is not active.'
        )}
      </p>
      <div className='mt-4 flex flex-wrap gap-2'>
        <Button
          variant='outline'
          className='min-h-11'
          disabled={props.busy}
          onClick={props.onResume}
        >
          {t('Resume original payment')}
        </Button>
        <Button
          variant='destructive'
          className='min-h-11'
          disabled={props.busy}
          onClick={props.onCancel}
        >
          {t('Cancel unpaid order')}
        </Button>
      </div>
    </section>
  )
}
