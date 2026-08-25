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
import { useLocation } from '@tanstack/react-router'
import { Megaphone } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { getStatus } from '@/lib/api'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useNotificationStore } from '@/stores/notification-store'

import {
  dismissPopupAnnouncements,
  getAnnouncementReadKey,
  getAnnouncementVersionKey,
  getPendingPopupAnnouncements,
  shouldShowAnnouncementPopupOnPath,
} from './lib/popup'
import type { SystemAnnouncement } from './types'

const announcementToneClass: Record<
  NonNullable<SystemAnnouncement['type']>,
  string
> = {
  default: 'bg-primary',
  ongoing: 'bg-sky-500',
  success: 'bg-emerald-500',
  warning: 'bg-amber-500',
  error: 'bg-rose-500',
}

function isSystemAnnouncement(value: unknown): value is SystemAnnouncement {
  return (
    value !== null &&
    typeof value === 'object' &&
    typeof (value as { content?: unknown }).content === 'string'
  )
}

export function AnnouncementPopup() {
  const { t } = useTranslation()
  const pathname = useLocation({ select: (location) => location.pathname })
  const allowedOnRoute = shouldShowAnnouncementPopupOnPath(pathname)
  const markAnnouncementsRead = useNotificationStore(
    (state) => state.markAnnouncementsRead
  )
  const [open, setOpen] = useState(false)
  const [visibleAnnouncements, setVisibleAnnouncements] = useState<
    SystemAnnouncement[]
  >([])

  const statusQuery = useQuery({
    queryKey: ['announcement-popup-status', pathname],
    queryFn: getStatus,
    enabled: allowedOnRoute,
    staleTime: 0,
    gcTime: 0,
    retry: 1,
    refetchOnMount: 'always',
    refetchOnWindowFocus: false,
  })

  const pendingAnnouncements = useMemo(() => {
    if (!statusQuery.isSuccess || statusQuery.isFetching || !allowedOnRoute) {
      return []
    }
    if (statusQuery.data?.announcements_enabled !== true) return []

    const source = statusQuery.data?.announcements
    if (!Array.isArray(source) || typeof window === 'undefined') return []

    return getPendingPopupAnnouncements(
      source.filter(isSystemAnnouncement),
      new Date(),
      window.localStorage,
      window.sessionStorage
    )
  }, [
    allowedOnRoute,
    statusQuery.data,
    statusQuery.isFetching,
    statusQuery.isSuccess,
  ])

  useEffect(() => {
    if (!allowedOnRoute) setOpen(false)
  }, [allowedOnRoute])

  useEffect(() => {
    if (!pendingAnnouncements.length) return
    setVisibleAnnouncements(pendingAnnouncements)
    setOpen(true)
  }, [pendingAnnouncements])

  const dismiss = () => {
    if (visibleAnnouncements.length > 0 && typeof window !== 'undefined') {
      dismissPopupAnnouncements(
        visibleAnnouncements,
        new Date(),
        window.localStorage,
        window.sessionStorage
      )
      markAnnouncementsRead(
        visibleAnnouncements.map((item) => getAnnouncementReadKey(item))
      )
    }
    setOpen(false)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      dismiss()
      return
    }
    setOpen(true)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={handleOpenChange}
      title={
        <span className='flex items-center gap-2'>
          <Megaphone className='text-primary size-5' aria-hidden='true' />
          {t('System Announcements')}
        </span>
      }
      description={t('{{count}} announcements require your attention', {
        count: visibleAnnouncements.length,
      })}
      contentClassName='benefit-liquid-glass max-sm:max-h-[calc(100dvh-1.5rem)] sm:max-w-2xl'
      contentHeight='min(62dvh, 620px)'
      bodyClassName='space-y-0'
      footer={
        <Button onClick={dismiss} className='min-h-11 sm:min-h-9'>
          {t('I understand')}
        </Button>
      }
    >
      <div className='divide-border divide-y'>
        {visibleAnnouncements.map((item) => {
          const tone = item.type ?? 'default'
          return (
            <section
              key={getAnnouncementVersionKey(item)}
              className='py-5 first:pt-1 last:pb-1'
            >
              <div className='mb-3 flex min-w-0 items-center gap-2'>
                <span
                  className={cn(
                    'size-2.5 shrink-0 rounded-full',
                    announcementToneClass[tone]
                  )}
                  aria-hidden='true'
                />
                {item.publishDate && (
                  <time className='text-muted-foreground text-xs'>
                    {formatDateTimeObject(new Date(item.publishDate))}
                  </time>
                )}
              </div>
              <RichContent breaks content={item.content} />
              {item.extra?.trim() && (
                <RichContent
                  breaks
                  content={item.extra}
                  className='text-muted-foreground mt-3 border-l-2 pl-3 text-xs'
                />
              )}
            </section>
          )
        })}
      </div>
    </Dialog>
  )
}
