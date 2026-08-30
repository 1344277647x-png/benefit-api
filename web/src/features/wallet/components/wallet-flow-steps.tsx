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
import {
  Check,
  CircleDollarSign,
  CreditCard,
  ReceiptText,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

interface WalletFlowStepsProps {
  currentStep: number
}

export function WalletFlowSteps(props: WalletFlowStepsProps) {
  const { t } = useTranslation()
  const steps = [
    { label: t('Balance'), icon: WalletCards },
    { label: t('Amount'), icon: CircleDollarSign },
    { label: t('Payment Method'), icon: CreditCard },
    { label: t('Confirm'), icon: ReceiptText },
  ]
  const currentStep = Math.min(steps.length - 1, Math.max(0, props.currentStep))

  return (
    <nav
      className='bg-card overflow-hidden rounded-2xl border shadow-xs'
      aria-label={t('Top up balance and view billing history.')}
    >
      <ol className='grid grid-cols-4'>
        {steps.map((step, index) => {
          const complete = index < currentStep
          const active = index === currentStep
          const Icon = complete ? Check : step.icon
          let state = 'upcoming'
          if (complete) state = 'complete'
          if (active) state = 'current'

          return (
            <li
              key={step.label}
              data-state={state}
              aria-current={active ? 'step' : undefined}
              className={cn(
                'relative flex min-w-0 flex-col items-center gap-1.5 border-r px-1.5 py-3 text-center last:border-r-0 sm:flex-row sm:justify-center sm:gap-2 sm:px-3 sm:py-3.5',
                active && 'bg-primary/[0.07]',
                complete && 'text-primary'
              )}
            >
              <span
                className={cn(
                  'bg-muted text-muted-foreground flex size-7 shrink-0 items-center justify-center rounded-full border sm:size-8',
                  active && 'border-primary/30 bg-primary/12 text-primary',
                  complete && 'border-primary/25 bg-primary/10 text-primary'
                )}
              >
                <Icon className='size-3.5 sm:size-4' aria-hidden='true' />
              </span>
              <span
                className={cn(
                  'text-muted-foreground min-w-0 text-[11px] leading-tight font-medium sm:text-xs',
                  (active || complete) && 'text-foreground'
                )}
              >
                {step.label}
              </span>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
