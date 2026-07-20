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

interface StatsProps {
  className?: string
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const highlights = [
    {
      value: 'One API',
      label: t('OpenAI-compatible access'),
    },
    {
      value: '100+',
      label: t('AI models supported'),
    },
    {
      value: t('Pay only for usage'),
      label: t('No subscription lock-in'),
    },
    {
      value: t('Billing history'),
      label: t('Balance, usage and orders'),
    },
  ]

  return (
    <section className='border-border/50 bg-muted/15 relative z-10 border-b'>
      <div className='mx-auto max-w-6xl px-5'>
        <div className='grid grid-cols-2 md:grid-cols-4'>
          {highlights.map((highlight) => (
            <div
              key={highlight.value}
              className='border-border/50 flex min-h-32 flex-col items-center justify-center border-b px-3 py-6 text-center even:border-l md:border-b-0 md:border-l md:first:border-l-0'
            >
              <p className='text-base leading-tight font-semibold md:text-lg'>
                {highlight.value}
              </p>
              <p className='text-muted-foreground mt-2 text-xs leading-relaxed'>
                {highlight.label}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
