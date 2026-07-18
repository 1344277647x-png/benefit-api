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
  CreditCard,
  Gauge,
  KeyRound,
  Languages,
  ReceiptText,
  Route,
  ShieldCheck,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

interface FeaturesProps {
  className?: string
}

export function Features(_props: FeaturesProps) {
  const { t } = useTranslation()

  const features = [
    {
      id: 'access',
      title: t('One endpoint, many models'),
      desc: t(
        'Use a single base URL across OpenAI-compatible tools and supported native routes.'
      ),
      icon: Route,
      className: 'text-teal-600 dark:text-teal-400',
      dotClassName: 'bg-teal-600 dark:bg-teal-400',
      detail: ['/v1/chat/completions', '/v1/responses', '/v1/messages'],
    },
    {
      id: 'routing',
      title: t('Stable routing by design'),
      desc: t(
        'Channel health, rate limits and request logs keep production traffic observable.'
      ),
      icon: ShieldCheck,
      className: 'text-sky-600 dark:text-sky-400',
      dotClassName: 'bg-sky-600 dark:bg-sky-400',
      detail: [t('Health checks'), t('Rate limits'), t('Usage logs')],
    },
    {
      id: 'billing',
      title: t('Usage and balance stay visible'),
      desc: t(
        'Recharge through the configured Epay channel and review every charge from the console.'
      ),
      icon: ReceiptText,
      className: 'text-amber-600 dark:text-amber-400',
      dotClassName: 'bg-amber-600 dark:bg-amber-400',
      detail: [t('Epay recharge'), t('Live balance'), t('Billing history')],
    },
  ]

  const additionalFeatures = [
    {
      icon: <Gauge className='size-5' strokeWidth={1.5} />,
      title: t('Performance metrics'),
      desc: t('See latency, usage and channel health in one console.'),
    },
    {
      icon: <KeyRound className='size-5' strokeWidth={1.5} />,
      title: t('Key controls'),
      desc: t('Set quotas, model access and expiration for each key.'),
    },
    {
      icon: <Languages className='size-5' strokeWidth={1.5} />,
      title: t('Chinese and English'),
      desc: t('Switch language without leaving your current workflow.'),
    },
    {
      icon: <CreditCard className='size-5' strokeWidth={1.5} />,
      title: t('Flexible recharge'),
      desc: t('Support Epay methods and redemption codes from one wallet.'),
    },
  ]

  return (
    <section className='relative z-10 px-6 py-20 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 max-w-xl'>
          <p className='text-muted-foreground mb-3 text-xs font-medium uppercase'>
            {t('Why Benefit API')}
          </p>
          <h2 className='text-2xl leading-tight font-bold md:text-3xl'>
            {t('A quieter way to manage AI access')}
          </h2>
        </AnimateInView>

        <div className='grid gap-4 md:grid-cols-3'>
          {features.map((f, i) => {
            const Icon = f.icon
            return (
              <AnimateInView
                key={f.id}
                delay={i * 100}
                animation='scale-in'
                className='bg-card border-border/60 rounded-[8px] border p-6 shadow-sm md:p-7'
              >
                <div className='bg-muted/70 mb-5 flex size-9 items-center justify-center rounded-[8px]'>
                  <Icon className={`size-5 ${f.className}`} />
                </div>
                <h3 className='text-base font-semibold'>{f.title}</h3>
                <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                  {f.desc}
                </p>
                <div className='mt-6 space-y-2 border-t pt-4'>
                  {f.detail.map((detail) => (
                    <div
                      key={detail}
                      className='text-muted-foreground flex items-center gap-2 font-mono text-xs'
                    >
                      <span
                        className={`size-1.5 rounded-full ${f.dotClassName}`}
                      />
                      <span className='truncate'>{detail}</span>
                    </div>
                  ))}
                </div>
              </AnimateInView>
            )
          })}
        </div>

        <div className='mt-14 grid grid-cols-1 gap-8 border-t pt-10 sm:grid-cols-2 md:grid-cols-4'>
          {additionalFeatures.map((f, i) => (
            <AnimateInView
              key={f.title}
              delay={i * 100}
              animation='fade-up'
              className='flex items-start gap-3'
            >
              <div className='text-muted-foreground mt-0.5 shrink-0'>
                {f.icon}
              </div>
              <div>
                <h3 className='mb-1.5 text-sm font-semibold'>{f.title}</h3>
                <p className='text-muted-foreground text-xs leading-relaxed'>
                  {f.desc}
                </p>
              </div>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
