import { Link } from '@tanstack/react-router'
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
  ArrowRight,
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
import { Button } from '@/components/ui/button'

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
    <section className='benefit-apple-shell relative z-10 pt-20 md:pt-24'>
      <div className='mx-auto max-w-6xl px-5'>
        <AnimateInView className='mx-auto max-w-2xl text-center'>
          <p className='text-muted-foreground mb-3 text-xs font-medium uppercase'>
            {t('Why Benefit API')}
          </p>
          <h2 className='text-2xl leading-tight font-bold md:text-3xl'>
            {t('A quieter way to manage AI access')}
          </h2>
        </AnimateInView>

        <div className='border-border/60 mt-12 grid border-y md:grid-cols-3 md:divide-x'>
          {features.map((f, i) => {
            const Icon = f.icon
            return (
              <AnimateInView
                key={f.id}
                delay={i * 100}
                animation='fade-up'
                className='border-border/60 border-b px-1 py-8 last:border-b-0 md:border-b-0 md:px-8'
              >
                <div className='bg-muted/70 mb-5 flex size-11 items-center justify-center rounded-[8px] border'>
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
      </div>

      <div className='bg-foreground text-background mt-20'>
        <div className='mx-auto grid max-w-6xl gap-8 px-5 py-14 md:grid-cols-[minmax(0,1fr)_auto] md:items-end md:py-16'>
          <AnimateInView className='max-w-3xl'>
            <p className='text-background/65 text-xs font-medium uppercase'>
              {t('Pay only for usage')}
            </p>
            <h2 className='mt-3 text-3xl leading-tight font-semibold md:text-5xl'>
              {t('Spend less on access. Build more.')}
            </h2>
            <p className='text-background/70 mt-5 max-w-2xl text-sm leading-relaxed md:text-base'>
              {t(
                'Recharge online, choose the model you need, and review every charge in the console.'
              )}
            </p>
          </AnimateInView>
          <Button
            className='benefit-liquid-inverse hover:bg-background/30 h-12 w-fit rounded-full px-6 font-semibold'
            render={<Link to='/pricing' />}
          >
            {t('View Pricing')}
            <ArrowRight className='ml-1.5 size-4' />
          </Button>
        </div>
      </div>

      <div className='mx-auto max-w-6xl px-5 py-16 md:py-20'>
        <div className='grid grid-cols-1 gap-8 sm:grid-cols-2 md:grid-cols-4'>
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
