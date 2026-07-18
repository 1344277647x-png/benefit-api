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
import { Braces, CreditCard, MessageSquareText, Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface StatsProps {
  className?: string
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const protocols = [
    {
      name: 'OpenAI',
      label: t('Compatible API'),
      icon: MessageSquareText,
      className: 'text-teal-600 dark:text-teal-400',
    },
    {
      name: 'Claude',
      label: t('Native message route'),
      icon: Braces,
      className: 'text-amber-600 dark:text-amber-400',
    },
    {
      name: 'Gemini',
      label: t('Native content route'),
      icon: Sparkles,
      className: 'text-sky-600 dark:text-sky-400',
    },
    {
      name: 'Epay',
      label: t('Online recharge flow'),
      icon: CreditCard,
      className: 'text-violet-600 dark:text-violet-400',
    },
  ]

  return (
    <div className='border-border/40 bg-muted/10 relative z-10 border-y'>
      <div className='mx-auto max-w-6xl px-6 py-10 md:py-12'>
        <div className='grid grid-cols-2 gap-x-6 gap-y-8 md:grid-cols-4 md:gap-10'>
          {protocols.map((protocol) => {
            const Icon = protocol.icon
            return (
              <div
                key={protocol.name}
                className='flex items-center justify-center gap-3 md:justify-start'
              >
                <Icon className={`size-5 shrink-0 ${protocol.className}`} />
                <div>
                  <p className='text-sm font-semibold'>{protocol.name}</p>
                  <p className='text-muted-foreground mt-0.5 text-xs'>
                    {protocol.label}
                  </p>
                </div>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
