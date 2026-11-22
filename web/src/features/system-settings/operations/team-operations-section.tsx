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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'

import { SettingsSection } from '../components/settings-section'

type LatePayment = {
  id: number
  team_id: number
  payer_user_id: number
  plan_title: string
  money: number
  trade_no: string
  complete_time: number
}

export function TeamOperationsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [evidence, setEvidence] = useState<Record<number, string>>({})
  const payments = useQuery({
    queryKey: ['team', 'late-payments'],
    queryFn: async () =>
      (await api.get('/api/team/admin/late-payments')).data
        .data as LatePayment[],
  })
  const resolve = useMutation({
    mutationFn: ({
      id,
      resolution,
    }: {
      id: number
      resolution: 'fulfil' | 'refunded'
    }) =>
      api.post(`/api/team/admin/late-payments/${id}/resolve`, {
        resolution,
        evidence_ref: evidence[id] || '',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ['team', 'late-payments'],
      })
      toast.success(t('Updated successfully'))
    },
    onError: () => toast.error(t('Request failed')),
  })
  return (
    <SettingsSection title={t('Late team payments')}>
      <p className='text-muted-foreground mb-4 text-sm'>
        {t(
          'Cancelled orders paid later never grant a plan automatically. Enter evidence before fulfilling or confirming a refund.'
        )}
      </p>
      <div className='space-y-3'>
        {(payments.data || []).map((payment) => (
          <div key={payment.id} className='space-y-3 rounded-xl border p-4'>
            <p>
              #{payment.id} · {t('Team')} #{payment.team_id} · {t('User')} #
              {payment.payer_user_id} · {payment.plan_title} ·{' '}
              {payment.money.toFixed(2)}
            </p>
            <Input
              value={evidence[payment.id] || ''}
              onChange={(event) =>
                setEvidence((current) => ({
                  ...current,
                  [payment.id]: event.target.value,
                }))
              }
              placeholder={t('Evidence reference')}
            />
            <div className='flex flex-wrap gap-2'>
              <Button
                disabled={resolve.isPending || !evidence[payment.id]}
                onClick={() =>
                  resolve.mutate({ id: payment.id, resolution: 'fulfil' })
                }
              >
                {t('Fulfil original plan')}
              </Button>
              <Button
                variant='outline'
                disabled={resolve.isPending || !evidence[payment.id]}
                onClick={() =>
                  resolve.mutate({ id: payment.id, resolution: 'refunded' })
                }
              >
                {t('Confirm refunded')}
              </Button>
            </div>
          </div>
        ))}
        {!payments.isPending && (payments.data || []).length === 0 && (
          <p>{t('No late payments awaiting resolution.')}</p>
        )}
      </div>
    </SettingsSection>
  )
}
