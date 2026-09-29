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

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { SectionPageLayout } from '@/components/layout'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getTopupInfo } from '@/features/wallet/api'
import { quotaUnitsToDollars } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { teamApi } from './api'
import { PendingTeamPayment } from './components/pending-team-payment'
import { availableTeamEpayMethods } from './lib/payment'
import type { TeamPlan } from './types'

type TeamAction =
  | { kind: 'create'; name: string }
  | { kind: 'invite'; email: string }
  | { kind: 'respond'; id: number; accept: boolean }
  | { kind: 'cancel'; id: number }
  | { kind: 'remove'; id: number }
  | { kind: 'disable'; id: number }
  | { kind: 'balance'; id: number }
  | { kind: 'cancel-payment' }
  | { kind: 'plan-dissolution' }
  | { kind: 'revoke-dissolution' }

const nameSchema = z.object({ name: z.string().trim().min(1).max(80) })
const inviteSchema = z.object({ email: z.email() })
const tokenSchema = z.object({
  name: z.string().trim().min(1).max(50),
  group: z.string().trim().min(1),
  model_limits: z.array(z.string()),
})

function Panel(props: { title: string; children: React.ReactNode }) {
  return (
    <section className='border-border/70 bg-card/75 min-w-0 rounded-2xl border p-4 sm:p-6'>
      <h2 className='mb-4 text-lg font-semibold'>{props.title}</h2>
      {props.children}
    </section>
  )
}

function PlanCard(props: {
  plan: TeamPlan
  paying: boolean
  methods: { type: string; name: string }[]
  onBalance: (id: number) => void
  onEpay: (id: number, method: string) => void
}) {
  const { t } = useTranslation()
  const [method, setMethod] = useState('')
  return (
    <div className='border-border/70 min-w-0 rounded-xl border p-4'>
      <h3 className='font-semibold'>{props.plan.title}</h3>
      <p className='text-muted-foreground mt-1 text-sm'>
        {t('Seats (including owner)')}: {props.plan.seat_limit} ·{' '}
        {t('Shared quota')}: {quotaUnitsToDollars(props.plan.total_amount)}
      </p>
      <p className='mt-2 text-sm'>
        {t('Price')}: {props.plan.price_amount.toFixed(2)} ·{' '}
        {props.plan.duration_value} {props.plan.duration_unit}
      </p>
      <div className='mt-4 flex flex-wrap gap-2'>
        {props.plan.allow_balance_pay !== false && (
          <Button
            className='min-h-11'
            disabled={props.paying}
            onClick={() => props.onBalance(props.plan.id)}
          >
            {t('Buy with balance')}
          </Button>
        )}
        {props.methods.length > 0 && (
          <>
            <label className='sr-only' htmlFor={`team-method-${props.plan.id}`}>
              {t('Payment method')}
            </label>
            <select
              id={`team-method-${props.plan.id}`}
              className='border-border bg-background min-h-11 rounded-lg border px-3'
              value={method}
              onChange={(event) => setMethod(event.target.value)}
            >
              <option value=''>{t('Payment method')}</option>
              {props.methods.map((item) => (
                <option key={item.type} value={item.type}>
                  {item.name}
                </option>
              ))}
            </select>
            <Button
              variant='outline'
              className='min-h-11'
              disabled={props.paying || !method}
              onClick={() => props.onEpay(props.plan.id, method)}
            >
              {t('Pay online')}
            </Button>
          </>
        )}
      </div>
    </div>
  )
}

export function TeamPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const userGroup = useAuthStore((state) => state.auth.user?.group || '')
  const [shownKey, setShownKey] = useState<string | null>(null)
  const [epayBusy, setEpayBusy] = useState(false)
  const self = useQuery({
    queryKey: ['team', 'self'],
    queryFn: teamApi.self,
    retry: false,
  })
  const isOwner = self.data?.membership?.role === 'owner'
  const plans = useQuery({
    queryKey: ['team', 'plans'],
    queryFn: teamApi.plans,
    enabled: isOwner,
    retry: false,
  })
  const tokens = useQuery({
    queryKey: ['team', 'tokens'],
    queryFn: teamApi.tokens,
    enabled: Boolean(self.data?.team),
    retry: false,
  })
  const groups = useQuery({
    queryKey: ['team', 'token-groups'],
    queryFn: teamApi.groups,
    enabled: Boolean(self.data?.team),
    retry: false,
  })
  const ownUsage = useQuery({
    queryKey: ['team', 'usage'],
    queryFn: teamApi.myUsage,
    enabled: Boolean(self.data?.team),
    retry: false,
  })
  const payment = useQuery({
    queryKey: ['team', 'payment'],
    queryFn: getTopupInfo,
    enabled: isOwner,
    retry: false,
  })
  const teamForm = useForm<z.infer<typeof nameSchema>>({
    resolver: zodResolver(nameSchema),
    defaultValues: { name: '' },
  })
  const inviteForm = useForm<z.infer<typeof inviteSchema>>({
    resolver: zodResolver(inviteSchema),
    defaultValues: { email: '' },
  })
  const tokenForm = useForm<z.infer<typeof tokenSchema>>({
    resolver: zodResolver(tokenSchema),
    defaultValues: { name: '', group: userGroup, model_limits: [] },
  })
  const selectedTokenGroup = tokenForm.watch('group')
  const tokenGroupOptions = useMemo(
    () =>
      Object.entries(groups.data || {})
        .filter(([group]) => group !== 'auto')
        .map(([value, info]) => ({ value, label: value, desc: info.desc })),
    [groups.data]
  )
  const tokenModels = useQuery({
    queryKey: ['team', 'token-models', selectedTokenGroup],
    queryFn: () => teamApi.models(selectedTokenGroup),
    enabled: Boolean(self.data?.team && selectedTokenGroup),
    retry: false,
  })

  useEffect(() => {
    if (tokenGroupOptions.length === 0) return
    const currentGroup = tokenForm.getValues('group')
    if (tokenGroupOptions.some((option) => option.value === currentGroup)) {
      return
    }
    const fallback =
      tokenGroupOptions.find((option) => option.value === userGroup)?.value ||
      tokenGroupOptions[0].value
    tokenForm.setValue('group', fallback)
    tokenForm.setValue('model_limits', [])
  }, [tokenForm, tokenGroupOptions, userGroup])

  const actions = useMutation({
    mutationFn: (action: TeamAction) => {
      switch (action.kind) {
        case 'create':
          return teamApi.create(action.name)
        case 'invite':
          return teamApi.invite(action.email)
        case 'respond':
          return teamApi.respond(action.id, action.accept)
        case 'cancel':
          return teamApi.cancel(action.id)
        case 'remove':
          return teamApi.remove(action.id)
        case 'disable':
          return teamApi.disableToken(action.id)
        case 'balance':
          return teamApi.balancePay(action.id)
        case 'cancel-payment':
          return teamApi.cancelPendingPayment()
        case 'plan-dissolution':
          return teamApi.planDissolution()
        case 'revoke-dissolution':
          return teamApi.revokeDissolution()
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['team'] })
      toast.success(t('Updated successfully'))
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: ['team', 'self'] })
      toast.error(t('Request failed'))
    },
  })
  const createToken = useMutation({
    mutationFn: (values: z.infer<typeof tokenSchema>) =>
      teamApi.createToken(values),
    onSuccess: (result) => {
      setShownKey(result.key)
      tokenForm.reset()
      void queryClient.invalidateQueries({ queryKey: ['team', 'tokens'] })
    },
    onError: () => toast.error(t('Request failed')),
  })

  const openEpayForm = async (
    request: () => ReturnType<typeof teamApi.epay>
  ) => {
    setEpayBusy(true)
    try {
      const result = await request()
      if (!result.url) {
        throw new Error('Missing gateway URL')
      }
      const target = new URL(result.url, window.location.origin)
      if (!['https:', 'http:'].includes(target.protocol)) {
        throw new Error('Invalid gateway URL')
      }
      const form = document.createElement('form')
      form.action = target.toString()
      form.method = 'POST'
      form.target = '_blank'
      for (const [name, value] of Object.entries(result.data || {})) {
        const input = document.createElement('input')
        input.type = 'hidden'
        input.name = name
        input.value = value
        form.appendChild(input)
      }
      document.body.appendChild(form)
      form.submit()
      form.remove()
      toast.success(t('Payment page opened'))
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setEpayBusy(false)
      void queryClient.invalidateQueries({ queryKey: ['team', 'self'] })
    }
  }

  const payEpay = async (planId: number, method: string) => {
    if (!window.confirm(t('Confirm team subscription purchase?'))) {
      return
    }
    await openEpayForm(() => teamApi.epay(planId, method))
  }

  const resumeEpay = async () => {
    if (
      !window.confirm(
        t('If you already paid, do not pay again. Resume the original payment?')
      )
    ) {
      return
    }
    await openEpayForm(teamApi.resumeEpay)
  }

  const data = self.data
  const membership = data?.membership
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Team subscription')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-5xl min-w-0 flex-col gap-4 pb-8'>
          {self.isPending && <p role='status'>{t('Loading...')}</p>}
          {self.isError && (
            <Panel title={t('Team subscription')}>
              <p role='alert'>
                {t(
                  'Team subscriptions are currently unavailable. No payment was made.'
                )}
              </p>
            </Panel>
          )}
          {data && (
            <>
              {data.invitations.length > 0 && (
                <Panel title={t('Pending invitations')}>
                  <div className='space-y-3'>
                    {data.invitations.map((invite) => (
                      <div
                        key={invite.id}
                        className='flex flex-wrap items-center justify-between gap-2'
                      >
                        <span>
                          {t('Team')} #{invite.team_id}
                        </span>
                        <div className='flex gap-2'>
                          <Button
                            className='min-h-11'
                            disabled={actions.isPending || Boolean(data.team)}
                            onClick={() =>
                              actions.mutate({
                                kind: 'respond',
                                id: invite.id,
                                accept: true,
                              })
                            }
                          >
                            {t('Accept')}
                          </Button>
                          <Button
                            variant='outline'
                            className='min-h-11'
                            disabled={actions.isPending}
                            onClick={() =>
                              actions.mutate({
                                kind: 'respond',
                                id: invite.id,
                                accept: false,
                              })
                            }
                          >
                            {t('Decline')}
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </Panel>
              )}
              {!data.team ? (
                <Panel title={t('Create a team')}>
                  <form
                    className='flex flex-col gap-3 sm:flex-row'
                    onSubmit={teamForm.handleSubmit((values) =>
                      actions.mutate({ kind: 'create', name: values.name })
                    )}
                  >
                    <label className='sr-only' htmlFor='team-name'>
                      {t('Team name')}
                    </label>
                    <Input
                      id='team-name'
                      className='min-h-11 min-w-0 flex-1'
                      placeholder={t('Team name')}
                      {...teamForm.register('name')}
                      aria-invalid={Boolean(teamForm.formState.errors.name)}
                    />
                    <Button
                      type='submit'
                      className='min-h-11'
                      disabled={actions.isPending}
                    >
                      {t('Create team')}
                    </Button>
                  </form>
                  {teamForm.formState.errors.name && (
                    <p role='alert'>
                      {t('Enter a team name (up to 80 characters).')}
                    </p>
                  )}
                </Panel>
              ) : (
                <>
                  <Panel title={data.team.name}>
                    <p>
                      {t('Your role')}:{' '}
                      {data.membership?.role === 'owner'
                        ? t('Owner')
                        : t('Member')}
                    </p>
                    {data.period && data.subscription ? (
                      <p className='mt-2'>
                        {t('Shared quota')}:{' '}
                        {quotaUnitsToDollars(
                          data.period.amount_total - data.period.amount_used
                        )}{' '}
                        / {quotaUnitsToDollars(data.period.amount_total)} ·{' '}
                        {t('Expires')}:{' '}
                        {new Date(
                          data.subscription.end_time * 1000
                        ).toLocaleDateString()}
                      </p>
                    ) : (
                      <p className='mt-2'>{t('No active team subscription')}</p>
                    )}
                    {ownUsage.data && (
                      <p className='text-muted-foreground mt-2 text-sm'>
                        {t('My team usage')}:{' '}
                        {quotaUnitsToDollars(ownUsage.data.amount)} ·{' '}
                        {ownUsage.data.requests} {t('requests')}
                      </p>
                    )}
                    {(data.queued || []).map((next) => (
                      <p
                        key={next.id}
                        className='text-muted-foreground mt-2 text-sm'
                      >
                        {t('Next term')}: {next.plan_title} ·{' '}
                        {new Date(next.start_time * 1000).toLocaleDateString()}
                      </p>
                    ))}
                  </Panel>
                  {membership?.role === 'owner' && data.pending_payment && (
                    <PendingTeamPayment
                      payment={data.pending_payment}
                      busy={epayBusy}
                      onResume={() => void resumeEpay()}
                      onCancel={() => {
                        if (window.confirm(t('Cancel this unpaid order?'))) {
                          actions.mutate({ kind: 'cancel-payment' })
                        }
                      }}
                    />
                  )}
                  <Panel title={t('My team keys')}>
                    <p className='text-muted-foreground mb-3 text-sm'>
                      {t(
                        'Team keys charge shared quota only; personal keys remain unchanged.'
                      )}
                    </p>
                    {shownKey && (
                      <div className='border-primary/50 mb-3 min-w-0 rounded-lg border p-3'>
                        <p>
                          {t('Copy this key now. It will not be shown again.')}
                        </p>
                        <code className='block break-all select-all'>
                          {shownKey}
                        </code>
                        <Button
                          type='button'
                          variant='outline'
                          className='mt-2 min-h-11'
                          onClick={() => setShownKey(null)}
                        >
                          {t('Dismiss key')}
                        </Button>
                      </div>
                    )}
                    <form
                      className='grid gap-3 sm:grid-cols-2'
                      onSubmit={tokenForm.handleSubmit((values) =>
                        createToken.mutate(values)
                      )}
                    >
                      <label className='sr-only' htmlFor='team-token-name'>
                        {t('Key name')}
                      </label>
                      <Input
                        id='team-token-name'
                        className='min-h-11 min-w-0 flex-1'
                        placeholder={t('Key name')}
                        {...tokenForm.register('name')}
                        aria-invalid={Boolean(tokenForm.formState.errors.name)}
                      />
                      <label className='sr-only' htmlFor='team-token-group'>
                        {t('Group')}
                      </label>
                      <select
                        id='team-token-group'
                        className='border-border bg-background min-h-11 min-w-0 rounded-lg border px-3'
                        value={selectedTokenGroup}
                        onChange={(event) => {
                          tokenForm.setValue('group', event.target.value, {
                            shouldDirty: true,
                            shouldValidate: true,
                          })
                          tokenForm.setValue('model_limits', [], {
                            shouldDirty: true,
                          })
                        }}
                        aria-invalid={Boolean(tokenForm.formState.errors.group)}
                      >
                        <option value='' disabled>
                          {t('Select a group')}
                        </option>
                        {tokenGroupOptions.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label} · {option.desc}
                          </option>
                        ))}
                      </select>
                      <div className='sm:col-span-2'>
                        <label
                          className='mb-1 block text-sm font-medium'
                          htmlFor='team-token-models'
                        >
                          {t('Model Limits')}
                        </label>
                        <MultiSelect
                          id='team-token-models'
                          options={(tokenModels.data || []).map((model) => ({
                            label: model,
                            value: model,
                          }))}
                          selected={tokenForm.watch('model_limits')}
                          onChange={(models) =>
                            tokenForm.setValue('model_limits', models, {
                              shouldDirty: true,
                              shouldValidate: true,
                            })
                          }
                          disabled={
                            !selectedTokenGroup || tokenModels.isPending
                          }
                          placeholder={t('Select models (empty for allow all)')}
                        />
                        <p className='text-muted-foreground mt-1 text-xs'>
                          {t('Limit which models can be used with this key')}
                        </p>
                      </div>
                      <Button
                        type='submit'
                        className='min-h-11 sm:col-span-2 sm:w-fit'
                        disabled={
                          createToken.isPending ||
                          !data.subscription ||
                          groups.isPending ||
                          groups.isError ||
                          tokenModels.isPending ||
                          tokenModels.isError
                        }
                      >
                        {t('Create team key')}
                      </Button>
                    </form>
                    {(tokenForm.formState.errors.name ||
                      tokenForm.formState.errors.group) && (
                      <p role='alert'>
                        {tokenForm.formState.errors.name
                          ? t('Enter a key name.')
                          : t('Select a group')}
                      </p>
                    )}
                    <div className='mt-4 space-y-2'>
                      {(tokens.data || []).map((token) => (
                        <div
                          key={token.id}
                          className='flex flex-wrap items-center justify-between gap-2'
                        >
                          <div className='min-w-0'>
                            <p className='break-all'>
                              {token.name} ·{' '}
                              {token.enabled ? t('Enabled') : t('Disabled')}
                            </p>
                            <p className='text-muted-foreground text-xs break-all'>
                              {t('Group')}: {token.group}
                              {' · '}
                              {t('Model Limits')}:{' '}
                              {token.model_limits_enabled
                                ? token.model_limits.join(', ')
                                : t('All Models')}
                            </p>
                          </div>
                          {token.enabled && (
                            <Button
                              variant='outline'
                              className='min-h-11'
                              disabled={actions.isPending}
                              onClick={() =>
                                actions.mutate({
                                  kind: 'disable',
                                  id: token.id,
                                })
                              }
                            >
                              {t('Disable key')}
                            </Button>
                          )}
                        </div>
                      ))}
                    </div>
                  </Panel>
                  {isOwner && (
                    <>
                      <Panel title={t('Team members')}>
                        <form
                          className='mb-4 flex flex-col gap-2 sm:flex-row'
                          onSubmit={inviteForm.handleSubmit((values) =>
                            actions.mutate({
                              kind: 'invite',
                              email: values.email,
                            })
                          )}
                        >
                          <label className='sr-only' htmlFor='team-email'>
                            {t('Member email')}
                          </label>
                          <Input
                            id='team-email'
                            type='email'
                            className='min-h-11 min-w-0 flex-1'
                            placeholder={t('Member email')}
                            {...inviteForm.register('email')}
                            aria-invalid={Boolean(
                              inviteForm.formState.errors.email
                            )}
                          />
                          <Button
                            type='submit'
                            className='min-h-11'
                            disabled={actions.isPending || !data.subscription}
                          >
                            {t('Invite member')}
                          </Button>
                        </form>
                        {inviteForm.formState.errors.email && (
                          <p role='alert'>
                            {t('Enter a valid email address.')}
                          </p>
                        )}
                        <div className='space-y-2'>
                          {(data.members || []).map((member) => (
                            <div
                              key={member.user_id}
                              className='flex flex-wrap items-center justify-between gap-2'
                            >
                              <span>
                                {t('User')} #{member.user_id} · {member.role}
                              </span>
                              {member.role !== 'owner' && (
                                <Button
                                  variant='outline'
                                  className='min-h-11'
                                  disabled={actions.isPending}
                                  onClick={() =>
                                    window.confirm(
                                      t(
                                        'Remove this member and revoke their team keys?'
                                      )
                                    ) &&
                                    actions.mutate({
                                      kind: 'remove',
                                      id: member.user_id,
                                    })
                                  }
                                >
                                  {t('Remove')}
                                </Button>
                              )}
                            </div>
                          ))}
                        </div>
                        {(data.sent_invitations || []).map((invite) => (
                          <div
                            key={invite.id}
                            className='mt-2 flex flex-wrap justify-between gap-2'
                          >
                            <span>
                              {invite.email} · {t('Pending')}
                            </span>
                            <Button
                              variant='outline'
                              className='min-h-11'
                              disabled={actions.isPending}
                              onClick={() =>
                                actions.mutate({
                                  kind: 'cancel',
                                  id: invite.id,
                                })
                              }
                            >
                              {t('Cancel invitation')}
                            </Button>
                          </div>
                        ))}
                      </Panel>
                      <Panel title={t('Member usage summary')}>
                        {(data.usage || []).length === 0 ? (
                          <p>{t('No team usage yet')}</p>
                        ) : (
                          (data.usage || []).map((row) => (
                            <p key={row.user_id} className='py-1'>
                              {t('User')} #{row.user_id}:{' '}
                              {quotaUnitsToDollars(row.amount)} · {row.requests}{' '}
                              {t('requests')}
                            </p>
                          ))
                        )}
                      </Panel>
                      <Panel title={t('Team plans')}>
                        <p className='text-muted-foreground mb-4 text-sm'>
                          {t(
                            'Early renewal starts after the current term. Unused quota does not carry over.'
                          )}
                        </p>
                        {plans.isPending && <p>{t('Loading...')}</p>}
                        {plans.isError && (
                          <p role='alert'>{t('Plans could not be loaded.')}</p>
                        )}
                        {plans.data?.length === 0 && (
                          <p>{t('No team plans are available.')}</p>
                        )}
                        <div className='grid gap-3 sm:grid-cols-2'>
                          {plans.data?.map((plan) => (
                            <PlanCard
                              key={plan.id}
                              plan={plan}
                              paying={
                                actions.isPending ||
                                epayBusy ||
                                Boolean(data.pending_payment) ||
                                (data.team?.dissolve_at ?? 0) > 0
                              }
                              methods={availableTeamEpayMethods(
                                payment.data?.data
                              )}
                              onBalance={(id) =>
                                window.confirm(
                                  t('Confirm team subscription purchase?')
                                ) && actions.mutate({ kind: 'balance', id })
                              }
                              onEpay={(id, method) => void payEpay(id, method)}
                            />
                          ))}
                        </div>
                      </Panel>
                      <Panel title={t('Team lifecycle')}>
                        {data.team.dissolve_at > 0 ? (
                          <>
                            <p>
                              {t(
                                'This team will be dissolved when the final paid term ends.'
                              )}{' '}
                              {new Date(
                                data.team.dissolve_at * 1000
                              ).toLocaleString()}
                            </p>
                            <Button
                              variant='outline'
                              className='mt-3 min-h-11'
                              disabled={actions.isPending}
                              onClick={() =>
                                actions.mutate({ kind: 'revoke-dissolution' })
                              }
                            >
                              {t('Revoke dissolution')}
                            </Button>
                          </>
                        ) : (
                          <Button
                            variant='destructive'
                            className='min-h-11'
                            disabled={
                              actions.isPending || Boolean(data.pending_payment)
                            }
                            onClick={() =>
                              window.confirm(
                                t(
                                  'Schedule dissolution at the end of the final paid term?'
                                )
                              ) && actions.mutate({ kind: 'plan-dissolution' })
                            }
                          >
                            {t('Schedule team dissolution')}
                          </Button>
                        )}
                      </Panel>
                    </>
                  )}
                  {membership?.role === 'member' && (
                    <Button
                      variant='outline'
                      className='min-h-11 self-start'
                      disabled={actions.isPending}
                      onClick={() =>
                        window.confirm(
                          t(
                            'Leave this team? Your team keys will stop working.'
                          )
                        ) &&
                        actions.mutate({
                          kind: 'remove',
                          id: membership.user_id,
                        })
                      }
                    >
                      {t('Leave team')}
                    </Button>
                  )}
                </>
              )}
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
