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
import { Link } from '@tanstack/react-router'
import { BookOpenText, Check, Copy, Server } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { cn } from '@/lib/utils'

import { getApiBaseUrl } from '../lib/server-address'

export function ApiEndpointBar() {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const apiBaseUrl = useMemo(() => {
    if (typeof window === 'undefined') return '/v1'
    return getApiBaseUrl()
  }, [])
  const copied = copiedText === apiBaseUrl

  return (
    <div className='benefit-cinema-panel bg-card flex flex-wrap items-center justify-between gap-2.5 rounded-xl border px-3.5 py-2.5'>
      <div className='flex min-w-0 flex-1 items-center gap-2'>
        <Server className='text-primary size-4 shrink-0' aria-hidden='true' />
        <span className='text-muted-foreground shrink-0 text-xs font-medium'>
          {t('API base URL')}
        </span>
        <code
          className='bg-muted/60 min-w-0 truncate rounded-md border px-2 py-0.5 font-mono text-xs'
          title={apiBaseUrl}
        >
          {apiBaseUrl}
        </code>
      </div>

      <div className='flex shrink-0 items-center gap-1.5'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='h-7 gap-1.5 px-2.5 text-xs'
          onClick={() => void copyToClipboard(apiBaseUrl)}
        >
          {copied ? (
            <Check className='size-3.5 text-emerald-600 dark:text-emerald-400' />
          ) : (
            <Copy className='size-3.5' />
          )}
          {copied ? t('Copied') : t('Copy URL')}
        </Button>
        <Link
          to='/docs'
          className={cn(
            buttonVariants({ variant: 'ghost', size: 'sm' }),
            'h-7 gap-1.5 px-2.5 text-xs'
          )}
        >
          <BookOpenText className='size-3.5' aria-hidden='true' />
          {t('Client setup guide')}
        </Link>
      </div>
    </div>
  )
}
