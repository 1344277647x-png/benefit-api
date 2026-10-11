import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'
import { getApiErrorMessage } from '@/lib/http-client'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

type Version = {
  id: number
  multiplier: string
  effective_at: number
  operator_id: number
}

export function ChannelCostSettings() {
  const { t } = useTranslation()
  const [channel, setChannel] = useState('')
  const [multiplier, setMultiplier] = useState('')
  const [effective, setEffective] = useState('')
  const queryClient = useQueryClient()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const channelId = Number(channel)
  const validChannel = Number.isSafeInteger(channelId) && channelId > 0
  const history = useQuery({
    queryKey: ['operations-cost-versions', channelId],
    enabled: validChannel,
    queryFn: async () => {
      const result = await api.get<{
        success: boolean
        message?: string
        data: Version[]
      }>('/api/log/operations-cost-versions', {
        params: { channel_id: channelId },
      })
      if (!result.data.success) {
        throw new Error(result.data.message || t('Request failed'))
      }
      return result.data.data
    },
  })
  const mutation = useMutation({
    mutationFn: async (input: {
      channel_id: number
      multiplier: string
      effective_at: number
    }) => {
      const result = await api.post<{ success: boolean; message?: string }>(
        '/api/log/operations-cost-versions',
        input
      )
      if (!result.data.success) {
        throw new Error(result.data.message || t('Request failed'))
      }
    },
    onSuccess: async (_, input) => {
      toast.success(t('Cost version saved'))
      await queryClient.invalidateQueries({
        queryKey: ['operations-cost-versions', input.channel_id],
      })
    },
    onError: (error) =>
      toast.error(getApiErrorMessage(error, t('Request failed'))),
  })
  const saving = mutation.isPending
  const save = () => {
    if (!isRoot || saving) return
    const at = effective ? Math.floor(new Date(effective).getTime() / 1000) : 0
    if (
      !validChannel ||
      !/^\d+(\.\d{1,8})?$/.test(multiplier) ||
      Number(multiplier) > 1000 ||
      (effective && (!Number.isFinite(at) || at <= Date.now() / 1000))
    ) {
      toast.error(
        t('Enter a valid channel, multiplier and future effective time.')
      )
      return
    }
    mutation.mutate({ channel_id: channelId, multiplier, effective_at: at })
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Upstream cost multipliers')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Cost equals base usage price times the upstream multiplier. Versions preserve historical costs. Root can save changes.'
          )}
        </p>
        <div className='grid gap-3 sm:grid-cols-3'>
          <label className='text-sm'>
            {t('Channel ID')}
            <Input
              disabled={saving}
              type='number'
              min={1}
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
            />
          </label>
          <label className='text-sm'>
            {t('Upstream multiplier')}
            <Input
              disabled={!isRoot || saving}
              inputMode='decimal'
              value={multiplier}
              onChange={(e) => setMultiplier(e.target.value)}
              placeholder='0.1'
            />
          </label>
          <label className='text-sm'>
            {t('Effective time (blank means now)')}
            <Input
              disabled={!isRoot || saving}
              type='datetime-local'
              value={effective}
              onChange={(e) => setEffective(e.target.value)}
            />
          </label>
        </div>
        <Button disabled={!isRoot || saving || !validChannel} onClick={save}>
          {t('Save cost version')}
        </Button>
        {history.isError && <p role='alert'>{t('Request failed')}</p>}
        {history.isFetching && <p>{t('Loading...')}</p>}
        {validChannel && !history.isFetching && history.data?.length === 0 && (
          <p>{t('No cost version configured; cost remains unknown.')}</p>
        )}
        <ul className='space-y-1 text-sm'>
          {history.data?.map((version) => (
            <li key={version.id}>
              {new Date(version.effective_at * 1000).toLocaleString()} ·{' '}
              {version.multiplier}× · {t('Operator ID')}: {version.operator_id}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
