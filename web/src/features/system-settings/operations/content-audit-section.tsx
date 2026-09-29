import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'

import { updateContentAuditSetting } from '../api'
import { SettingsSection } from '../components/settings-section'

type AuditRow = {
  id: number
  user_id: number
  team_id: number
  model_name: string
  request_id: string
  source: string
  status: string
  created_at: number
  input_truncated: boolean
  output_truncated: boolean
}
type AuditDetail = AuditRow & {
  input: string
  output: string
  result_references: string
}

export function ContentAuditSection(props: {
  enabled: boolean
  privacyReady: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [privacyReady, setPrivacyReady] = useState(props.privacyReady)
  const [userID, setUserID] = useState('')
  const [model, setModel] = useState('')
  const [requestID, setRequestID] = useState('')
  const [source, setSource] = useState('')
  const [status, setStatus] = useState('')
  const [startAt, setStartAt] = useState('')
  const [endAt, setEndAt] = useState('')
  const [selected, setSelected] = useState<number | null>(null)
  const save = useMutation({
    mutationFn: (enabled: boolean) =>
      updateContentAuditSetting({ enabled, privacy_ready: privacyReady }),
    onSuccess: (response) => {
      if (!response.success) throw new Error(response.message)
      void queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Setting updated successfully'))
    },
    onError: (error: Error) => toast.error(error.message),
  })
  const list = useQuery({
    queryKey: [
      'content-audit',
      userID,
      model,
      requestID,
      source,
      status,
      startAt,
      endAt,
    ],
    queryFn: async () =>
      (
        await api.get('/api/content-audit/', {
          params: {
            user_id: userID || undefined,
            model: model || undefined,
            request_id: requestID || undefined,
            source: source || undefined,
            status: status || undefined,
            start_at: startAt
              ? Math.floor(new Date(startAt).getTime() / 1000)
              : undefined,
            end_at: endAt
              ? Math.floor(new Date(endAt).getTime() / 1000)
              : undefined,
          },
        })
      ).data.data as { items: AuditRow[] },
  })
  const detail = useQuery({
    queryKey: ['content-audit', selected],
    queryFn: async () =>
      (await api.get(`/api/content-audit/${selected}`)).data
        .data as AuditDetail,
    enabled: selected !== null,
  })

  return (
    <SettingsSection title={t('Content audit')}>
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Stores redacted user input and final visible results encrypted for 7 days. Hidden reasoning, system prompts and media binaries are never stored.'
          )}
        </p>
        <label className='flex items-center gap-2 text-sm'>
          <input
            type='checkbox'
            checked={privacyReady}
            onChange={(event) => setPrivacyReady(event.target.checked)}
          />
          {t('Privacy policy is ready')}
        </label>
        <div className='flex items-center gap-3'>
          <Switch
            checked={props.enabled}
            disabled={save.isPending || (!privacyReady && !props.enabled)}
            onCheckedChange={(enabled) => save.mutate(enabled)}
          />
          <span>{props.enabled ? t('Enabled') : t('Disabled')}</span>
        </div>
        <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-3'>
          <Input
            value={userID}
            onChange={(event) => setUserID(event.target.value)}
            placeholder={t('User ID')}
            aria-label={t('User ID')}
          />
          <Input
            value={model}
            onChange={(event) => setModel(event.target.value)}
            placeholder={t('Model')}
            aria-label={t('Model')}
          />
          <Input
            value={requestID}
            onChange={(event) => setRequestID(event.target.value)}
            placeholder={t('Request ID')}
            aria-label={t('Request ID')}
          />
          <Input
            value={source}
            onChange={(event) => setSource(event.target.value)}
            placeholder={t('Source')}
            aria-label={t('Source')}
          />
          <Input
            value={status}
            onChange={(event) => setStatus(event.target.value)}
            placeholder={t('Status')}
            aria-label={t('Status')}
          />
          <Input
            type='datetime-local'
            value={startAt}
            onChange={(event) => setStartAt(event.target.value)}
            aria-label={t('Start Time')}
          />
          <Input
            type='datetime-local'
            value={endAt}
            onChange={(event) => setEndAt(event.target.value)}
            aria-label={t('End Time')}
          />
        </div>
        <div className='space-y-2'>
          {(list.data?.items || []).map((row) => (
            <button
              key={row.id}
              type='button'
              className='border-border hover:bg-muted/40 w-full rounded-lg border p-3 text-left text-sm'
              onClick={() => setSelected(row.id)}
            >
              #{row.id} · {row.source} · {row.model_name} · {row.request_id} ·{' '}
              {row.status}
            </button>
          ))}
        </div>
        {detail.data && (
          <div className='space-y-3 rounded-xl border p-4'>
            <div className='flex justify-between'>
              <strong>{t('Audit detail')}</strong>
              <Button variant='outline' onClick={() => setSelected(null)}>
                {t('Close')}
              </Button>
            </div>
            <div>
              <p className='text-muted-foreground text-xs'>{t('Input')}</p>
              <pre className='max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap'>
                {detail.data.input}
              </pre>
            </div>
            <div>
              <p className='text-muted-foreground text-xs'>
                {t('Final output')}
              </p>
              <pre className='max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap'>
                {detail.data.output}
              </pre>
            </div>
            {detail.data.result_references && (
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Result references')}
                </p>
                <pre className='text-xs break-all'>
                  {detail.data.result_references}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    </SettingsSection>
  )
}
