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
import { KeyRound, PlugZap, UserPlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

export function HowItWorks() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '1',
      title: t('Create your account'),
      desc: t('Register, sign in and add balance through the wallet.'),
      icon: <UserPlus className='size-6' strokeWidth={1.5} />,
    },
    {
      num: '2',
      title: t('Create an API key'),
      desc: t('Choose allowed models, quota and expiration for the key.'),
      icon: <KeyRound className='size-6' strokeWidth={1.5} />,
    },
    {
      num: '3',
      title: t('Connect your client'),
      desc: t('Use the Benefit API base URL and start sending requests.'),
      icon: <PlugZap className='size-6' strokeWidth={1.5} />,
    },
  ]

  return (
    <section className='border-border/40 bg-muted/10 relative z-10 border-y px-6 py-20 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-16 text-center md:mb-20'>
          <p className='text-muted-foreground mb-3 text-xs font-medium uppercase'>
            {t('How It Works')}
          </p>
          <h2 className='text-2xl font-bold md:text-3xl'>
            {t('Three steps to get started')}
          </h2>
        </AnimateInView>

        <div className='grid gap-8 md:grid-cols-3 md:gap-12'>
          {steps.map((step, i) => (
            <AnimateInView
              key={step.num}
              delay={i * 150}
              animation='fade-up'
              className='relative flex flex-col items-center text-center'
            >
              <div className='relative mb-6'>
                <div className='text-muted-foreground border-border/50 bg-background flex size-16 items-center justify-center rounded-[8px] border transition-colors'>
                  {step.icon}
                </div>
                <div className='bg-foreground text-background absolute -top-2 -right-2 flex size-6 items-center justify-center rounded-full text-xs font-bold'>
                  {step.num}
                </div>
              </div>
              <h3 className='mb-2 text-base font-semibold'>{step.title}</h3>
              <p className='text-muted-foreground max-w-[240px] text-sm leading-relaxed'>
                {step.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}
