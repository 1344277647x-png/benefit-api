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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { BookOpenText } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Skeleton } from '@/components/ui/skeleton'

import { getDocsContent } from './api'
import { ConfigurationPanel } from './configuration-panel'

function getApiBaseUrl(): string {
  if (typeof window === 'undefined') return '/v1'
  return `${window.location.origin}/v1`
}

function DocsEmptyState({ loadFailed = false }: { loadFailed?: boolean }) {
  const { t } = useTranslation()

  return (
    <div className='flex min-h-[52vh] items-center justify-center py-12'>
      <div className='max-w-lg space-y-4 text-center'>
        <div className='bg-muted mx-auto flex size-12 items-center justify-center rounded-lg'>
          <BookOpenText className='text-muted-foreground size-6' />
        </div>
        <div className='space-y-1.5'>
          <h1 className='text-xl font-semibold'>{t('Docs')}</h1>
          <p className='text-muted-foreground text-sm leading-relaxed'>
            {loadFailed
              ? t('Failed to load documentation.')
              : t('The documentation is not configured yet.')}
          </p>
          {!loadFailed && (
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {t(
                'Add Markdown content in Site Settings to publish your guide.'
              )}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

export function Docs() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['docs-content'],
    queryFn: getDocsContent,
    staleTime: 60 * 1000,
  })

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-9 w-40' />
          <Skeleton className='h-5 w-[75%]' />
          <Skeleton className='h-5 w-full' />
          <Skeleton className='h-5 w-[88%]' />
        </div>
      </PublicLayout>
    )
  }

  const content = data?.data?.trim() ?? ''
  if (isError || !data?.success || !content) {
    return (
      <PublicLayout>
        <div className='mx-auto max-w-4xl py-8 md:py-12'>
          <ConfigurationPanel apiBaseUrl={getApiBaseUrl()} />
          <DocsEmptyState loadFailed={isError || data?.success === false} />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout>
      <div className='mx-auto max-w-4xl py-8 md:py-12'>
        <article>
          <header className='mb-8 border-b pb-5'>
            <h1 className='text-3xl font-semibold'>{t('Docs')}</h1>
          </header>
          <ConfigurationPanel apiBaseUrl={getApiBaseUrl()} />
          <RichContent
            mode='markdown'
            content={content}
            className='prose-neutral dark:prose-invert max-w-none'
          />
        </article>
        <p className='text-muted-foreground mt-8 text-center text-xs'>
          {t('Need pricing or model compatibility details?')}{' '}
          <Link
            to='/pricing'
            className='text-foreground underline underline-offset-4'
          >
            {t('Open Model Square')}
          </Link>
        </p>
      </div>
    </PublicLayout>
  )
}
