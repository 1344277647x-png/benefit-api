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
import { ArrowUp, BookOpenText, ListTree } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { getApiBaseUrl } from '@/features/keys/lib/server-address'
import type { MarkdownHeading } from '@/lib/markdown-headings'
import { cn } from '@/lib/utils'

import { getDocsContent } from './api'
import { ConfigurationPanel } from './configuration-panel'

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
  const content = data?.data?.trim() ?? ''
  const articleRef = useRef<HTMLElement>(null)
  const [headings, setHeadings] = useState<MarkdownHeading[]>([])
  const [activeHeadingId, setActiveHeadingId] = useState<string>('')

  useEffect(() => {
    if (!articleRef.current || headings.length === 0) return
    const elements = [
      ...articleRef.current.querySelectorAll<HTMLElement>(
        '[data-docs-heading]'
      ),
    ]
    const target = elements.find(
      (element) =>
        `#${encodeURIComponent(element.id)}` === window.location.hash ||
        `#${element.id}` === window.location.hash
    )
    target?.scrollIntoView()
    let frame: number | undefined
    const updateActiveHeading = () => {
      if (frame !== undefined) return
      frame = window.requestAnimationFrame(() => {
        frame = undefined
        let active = elements[0]
        for (const element of elements) {
          if (element.getBoundingClientRect().top > 100) break
          active = element
        }
        setActiveHeadingId(active?.id ?? '')
      })
    }
    updateActiveHeading()
    window.addEventListener('scroll', updateActiveHeading, { passive: true })
    window.addEventListener('resize', updateActiveHeading)
    return () => {
      if (frame !== undefined) window.cancelAnimationFrame(frame)
      window.removeEventListener('scroll', updateActiveHeading)
      window.removeEventListener('resize', updateActiveHeading)
    }
  }, [headings])

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

  const hasToc = headings.length >= 2
  const currentActiveId = headings.some(
    (heading) => heading.id === activeHeadingId
  )
    ? activeHeadingId
    : headings[0]?.id

  const scrollToTop = () => {
    window.scrollTo({ top: 0, behavior: 'auto' })
  }

  return (
    <PublicLayout>
      <div
        className={cn(
          'mx-auto py-8 md:py-12',
          hasToc ? 'max-w-6xl' : 'max-w-4xl'
        )}
      >
        <div
          className={cn(
            hasToc &&
              'xl:grid xl:grid-cols-[minmax(0,1fr)_15rem] xl:items-start xl:gap-8'
          )}
        >
          <article ref={articleRef} className='min-w-0'>
            <header className='mb-8 border-b pb-5'>
              <h1 className='text-3xl font-semibold'>{t('Docs')}</h1>
            </header>
            <ConfigurationPanel apiBaseUrl={getApiBaseUrl()} />

            {hasToc && (
              <details className='bg-card/70 mb-6 rounded-xl border px-4 py-3 xl:hidden'>
                <summary className='flex cursor-pointer items-center gap-2 text-sm font-medium select-none'>
                  <ListTree className='text-muted-foreground size-4' />
                  <span>{t('On this page')}</span>
                </summary>
                <nav
                  aria-label={t('On this page')}
                  className='mt-3 border-t pt-2.5'
                >
                  <ul className='space-y-1.5 text-sm'>
                    {headings.map((heading) => (
                      <li
                        key={heading.id}
                        className={cn(heading.level === 3 && 'pl-3.5')}
                      >
                        <a
                          href={`#${encodeURIComponent(heading.id)}`}
                          aria-current={
                            currentActiveId === heading.id
                              ? 'location'
                              : undefined
                          }
                          onClick={() => setActiveHeadingId(heading.id)}
                          className={cn(
                            'hover:text-foreground block truncate py-0.5 transition-colors',
                            currentActiveId === heading.id
                              ? 'text-primary font-medium'
                              : 'text-muted-foreground'
                          )}
                        >
                          {heading.text}
                        </a>
                      </li>
                    ))}
                  </ul>
                </nav>
              </details>
            )}

            <RichContent
              mode='markdown'
              content={content}
              onHeadingsChange={setHeadings}
              className='prose-neutral dark:prose-invert max-w-none'
            />
          </article>

          {hasToc && (
            <aside className='sticky top-20 hidden xl:block'>
              <div className='bg-card/60 rounded-xl border p-4'>
                <div className='mb-2.5 flex items-center gap-2 text-xs font-semibold tracking-wide uppercase'>
                  <ListTree className='text-muted-foreground size-3.5' />
                  <span>{t('On this page')}</span>
                </div>
                <nav aria-label={t('On this page')}>
                  <ul className='max-h-[65vh] space-y-1.5 overflow-y-auto pr-1 text-xs'>
                    {headings.map((heading) => (
                      <li
                        key={heading.id}
                        className={cn(heading.level === 3 && 'pl-3')}
                      >
                        <a
                          href={`#${encodeURIComponent(heading.id)}`}
                          aria-current={
                            currentActiveId === heading.id
                              ? 'location'
                              : undefined
                          }
                          onClick={() => setActiveHeadingId(heading.id)}
                          className={cn(
                            'hover:text-foreground block truncate py-0.5 transition-colors',
                            currentActiveId === heading.id
                              ? 'text-primary font-medium'
                              : 'text-muted-foreground'
                          )}
                          title={heading.text}
                        >
                          {heading.text}
                        </a>
                      </li>
                    ))}
                  </ul>
                </nav>
                <div className='mt-3 border-t pt-2.5'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='xs'
                    className='text-muted-foreground hover:text-foreground w-full justify-start gap-1.5'
                    onClick={scrollToTop}
                  >
                    <ArrowUp className='size-3.5' />
                    {t('Back to top')}
                  </Button>
                </div>
              </div>
            </aside>
          )}
        </div>

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
