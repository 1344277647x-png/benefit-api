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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Input } from '@/components/ui/input'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'

import { getUsers, searchUsers } from '../api'
import {
  USER_STATUS,
  getUserStatusOptions,
  getUserRoleOptions,
  isUserDeleted,
} from '../constants'
import type { User } from '../types'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { useUsersColumns } from './users-columns'
import { useUsers } from './users-provider'

const route = getRouteApi('/_authenticated/users/')

function isDisabledUserRow(user: User) {
  return isUserDeleted(user) || user.status === USER_STATUS.DISABLED
}

export function UsersTable() {
  const { t } = useTranslation()
  const columns = useUsersColumns()
  const { refreshTrigger } = useUsers()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [usageRange, setUsageRange] = useState<UsageRange>('lifetime')
  const [customStart, setCustomStart] = useState('')
  const [customEnd, setCustomEnd] = useState('')

  const usageParams = useMemo(() => {
    const now = Math.floor(Date.now() / 1000)
    if (usageRange === 'lifetime') return {}
    if (usageRange === 'custom') {
      const start = customStart
        ? Math.floor(new Date(`${customStart}T00:00:00`).getTime() / 1000)
        : undefined
      const end = customEnd
        ? Math.floor(new Date(`${customEnd}T23:59:59`).getTime() / 1000)
        : undefined
      return { usage_start: start, usage_end: end }
    }
    let days = 30
    if (usageRange === '24h') days = 1
    else if (usageRange === '7d') days = 7
    return { usage_start: now - days * 86400, usage_end: now }
  }, [customEnd, customStart, usageRange])

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'role', searchKey: 'role', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'string' },
    ],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'status')?.value as
      | string[]
      | undefined) ?? []
  const roleFilter =
    (columnFilters.find((filter) => filter.id === 'role')?.value as
      | string[]
      | undefined) ?? []
  const groupFilter =
    (columnFilters.find((filter) => filter.id === 'group')?.value as string) ??
    ''

  // Fetch data with React Query
  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'users',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
      roleFilter,
      groupFilter,
      usageRange,
      usageParams.usage_start,
      usageParams.usage_end,
      refreshTrigger,
    ],
    queryFn: async () => {
      const hasFilter = globalFilter?.trim()
      const hasColumnFilter =
        statusFilter.length > 0 || roleFilter.length > 0 || Boolean(groupFilter)
      const params = {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }

      const result =
        hasFilter || hasColumnFilter
          ? await searchUsers({
              ...params,
              keyword: globalFilter,
              status: statusFilter[0] ?? '',
              role: roleFilter[0] ?? '',
              group: groupFilter,
              ...usageParams,
            })
          : await getUsers({ ...params, ...usageParams })

      if (!result.success) {
        toast.error(
          result.message || `Failed to ${hasFilter ? 'search' : 'load'} users`
        )
        return { items: [], total: 0 }
      }

      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const users = data?.items || []

  const { table } = useDataTable({
    data: users,
    columns,
    enableRowSelection: true,
    columnFilters,
    globalFilter,
    pagination,
    globalFilterFn: (row, _columnId, filterValue) => {
      const searchValue = String(filterValue).toLowerCase()
      const fields = [
        row.getValue('username'),
        row.original.display_name,
        row.original.email,
      ]
      return fields.some((field) =>
        String(field || '')
          .toLowerCase()
          .includes(searchValue)
      )
    },
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Users Found')}
      emptyDescription={t(
        'No users available. Try adjusting your search or filters.'
      )}
      skeletonKeyPrefix='users-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by username, name or email...'),
        additionalSearch: (
          <div className='flex flex-wrap items-center gap-1.5'>
            <select
              value={usageRange}
              onChange={(event) =>
                setUsageRange(event.target.value as UsageRange)
              }
              className='border-input bg-background h-8 rounded-lg border px-2 text-xs outline-none'
              aria-label={t('Usage range')}
            >
              <option value='24h'>{t('Last 24 hours')}</option>
              <option value='7d'>{t('Last 7 days')}</option>
              <option value='30d'>{t('Last 30 days')}</option>
              <option value='lifetime'>{t('All time')}</option>
              <option value='custom'>{t('Custom range')}</option>
            </select>
            {usageRange === 'custom' && (
              <>
                <Input
                  type='date'
                  value={customStart}
                  onChange={(event) => setCustomStart(event.target.value)}
                  className='h-8 w-[132px] text-xs'
                  aria-label={t('Start date')}
                />
                <Input
                  type='date'
                  value={customEnd}
                  onChange={(event) => setCustomEnd(event.target.value)}
                  className='h-8 w-[132px] text-xs'
                  aria-label={t('End date')}
                />
              </>
            )}
          </div>
        ),
        hasAdditionalFilters:
          usageRange !== 'lifetime' || Boolean(customStart || customEnd),
        onReset: () => {
          setUsageRange('lifetime')
          setCustomStart('')
          setCustomEnd('')
        },
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: getUserStatusOptions(t),
            singleSelect: true,
          },
          {
            columnId: 'role',
            title: t('Role'),
            options: getUserRoleOptions(t),
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, { isMobile }) => {
        if (!isDisabledUserRow(row.original)) return undefined
        return isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
      }}
      bulkActions={<DataTableBulkActions table={table} />}
    />
  )
}

type UsageRange = '24h' | '7d' | '30d' | 'lifetime' | 'custom'
