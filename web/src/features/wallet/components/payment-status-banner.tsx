import { Link } from '@tanstack/react-router'
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
import { ArrowRight, CheckCircle2, Clock3, X, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import type { PaymentReturnStatus } from '../constants'

interface PaymentStatusBannerProps {
  status: PaymentReturnStatus
  onDismiss: () => void
  onOpenHistory: () => void
}

export function PaymentStatusBanner(props: PaymentStatusBannerProps) {
  const { t } = useTranslation()

  let Icon = Clock3
  let title = t('Payment is being confirmed')
  let description = t(
    'The provider has returned you to Benefit API. The balance will update after the callback is verified.'
  )
  let className = 'border-warning/40 bg-warning/5'

  if (props.status === 'success') {
    Icon = CheckCircle2
    title = t('Payment completed')
    description = t(
      'Your payment callback was accepted. Refreshing the balance and keeping the order record available below.'
    )
    className = 'border-success/40 bg-success/5'
  } else if (props.status === 'fail') {
    Icon = XCircle
    title = t('Payment failed')
    description = t(
      'The payment was not completed. Check the order history for the provider response, then try another method.'
    )
    className = 'border-destructive/40 bg-destructive/5'
  } else if (props.status === 'cancelled') {
    Icon = XCircle
    title = t('Payment cancelled')
    description = t(
      'No balance was added. You can return to the amount selector and start a new payment when ready.'
    )
    className = 'border-border bg-muted/30'
  }

  return (
    <Alert className={className}>
      <Icon className='size-4' aria-hidden='true' />
      <div className='min-w-0'>
        <div className='flex items-start justify-between gap-3'>
          <AlertTitle>{title}</AlertTitle>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            className='-mt-1 -mr-1 shrink-0'
            onClick={props.onDismiss}
            aria-label={t('Dismiss')}
          >
            <X aria-hidden='true' />
          </Button>
        </div>
        <AlertDescription className='mt-1'>{description}</AlertDescription>
        <div className='mt-3 flex flex-wrap gap-2'>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={props.onOpenHistory}
          >
            {t('View Order History')}
          </Button>
          {(props.status === 'success' || props.status === 'pending') && (
            <Button
              type='button'
              size='sm'
              variant='ghost'
              render={
                <Link
                  to='/playground'
                  className='inline-flex items-center gap-1.5'
                >
                  {t('Continue calling')}
                  <ArrowRight className='size-3.5' aria-hidden='true' />
                </Link>
              }
            />
          )}
        </div>
      </div>
    </Alert>
  )
}
