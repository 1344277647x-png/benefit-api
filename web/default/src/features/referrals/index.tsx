import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Check,
  CheckCircle2,
  Clock3,
  Gift,
  History,
  Loader2,
  Percent,
  Plus,
  Share2,
  UserPlus,
  UsersRound,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getSelf } from '@/lib/api'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { TransferDialog } from '../wallet/components/dialogs/transfer-dialog'
import {
  getReferralInvitees,
  getReferralOverview,
  getReferralRewards,
  transferReferralQuota,
} from './api'
import type {
  ReferralInviteeItem,
  ReferralOverview,
  ReferralPage,
  ReferralRewardItem,
  ReferralRules,
} from './types'

function queryError(response: { success?: boolean; message?: string }) {
  if (response.success === false) {
    throw new Error(response.message || 'Unable to load referral data')
  }
}

function ReferralStat({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: LucideIcon
  label: string
  value: string
  tone: 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4'
}) {
  const toneClasses = {
    'chart-1': 'bg-chart-1/12 text-chart-1',
    'chart-2': 'bg-chart-2/12 text-chart-2',
    'chart-3': 'bg-chart-3/12 text-chart-3',
    'chart-4': 'bg-chart-4/12 text-chart-4',
  } as const
  return (
    <div className='border-border/60 bg-background/50 min-w-0 rounded-xl border p-3'>
      <div className='text-muted-foreground flex items-center gap-2'>
        <span className={`rounded-lg p-1.5 ${toneClasses[tone]}`}>
          <Icon className='size-4' aria-hidden='true' />
        </span>
        <span className='min-w-0 truncate text-xs font-medium'>{label}</span>
      </div>
      <p className='mt-2 text-xl font-semibold break-words tabular-nums'>
        {value}
      </p>
    </div>
  )
}

function ReferralRulesList({ rules }: { rules: ReferralRules }) {
  const { t } = useTranslation()
  const rate = `${(rules.reward_rate_basis_points / 100).toFixed(2)}%`
  return (
    <ul className='text-muted-foreground grid gap-2 text-sm sm:grid-cols-2'>
      <li>
        {t('A valid first top-up must reach {{amount}}.', {
          amount: formatQuota(rules.minimum_topup_quota),
        })}
      </li>
      <li>{t('Inviter reward rate: {{rate}}.', { rate })}</li>
      <li>
        {t('Invitee bonus: {{amount}}.', {
          amount: formatQuota(rules.invitee_bonus_quota),
        })}
      </li>
      <li>
        {t('Settlement delay: {{hours}} hours.', {
          hours: rules.settlement_delay_hours,
        })}
      </li>
      {rules.per_invitee_cap_quota > 0 && (
        <li>
          {t('Per invitee cap: {{amount}}.', {
            amount: formatQuota(rules.per_invitee_cap_quota),
          })}
        </li>
      )}
      {rules.monthly_cap_quota > 0 && (
        <li>
          {t('Monthly cap: {{amount}}.', {
            amount: formatQuota(rules.monthly_cap_quota),
          })}
        </li>
      )}
    </ul>
  )
}

function formatRewardRate(rules: ReferralRules | undefined) {
  if (!rules || !Number.isFinite(rules.reward_rate_basis_points)) return '-'
  return `${(rules.reward_rate_basis_points / 100).toFixed(2)}%`
}

function ReferralStep({
  number,
  icon: Icon,
  title,
  description,
}: {
  number: string
  icon: LucideIcon
  title: string
  description: string
}) {
  return (
    <div className='flex min-w-0 gap-3'>
      <span className='bg-primary/12 text-primary flex size-9 shrink-0 items-center justify-center rounded-full text-xs font-semibold tabular-nums'>
        {number}
      </span>
      <div className='min-w-0'>
        <div className='flex items-center gap-2'>
          <Icon className='text-primary size-4 shrink-0' aria-hidden='true' />
          <p className='text-sm font-medium'>{title}</p>
        </div>
        <p className='text-muted-foreground mt-1 text-xs leading-5'>
          {description}
        </p>
      </div>
    </div>
  )
}

function ReferralRewardRows({
  loading,
  data,
}: {
  loading: boolean
  data: ReferralPage<ReferralRewardItem> | undefined
}) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <TableRow>
        <TableCell colSpan={4}>
          <Loader2 className='text-muted-foreground mx-auto size-5 animate-spin' />
        </TableCell>
      </TableRow>
    )
  }
  if (!data?.items.length) {
    return (
      <TableRow>
        <TableCell
          colSpan={4}
          className='text-muted-foreground h-24 text-center'
        >
          {t('No rewards yet')}
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.map((item) => {
        let statusLabel = t('Pending')
        if (item.status === 'settled') statusLabel = t('Settled')
        if (item.status === 'capped') statusLabel = t('Capped')
        const StatusIcon = item.status === 'settled' ? Check : Clock3
        return (
          <TableRow key={`${item.created_at}-${item.invitee_username}`}>
            <TableCell className='font-medium'>
              {item.invitee_username}
            </TableCell>
            <TableCell>{formatQuota(item.reward_quota)}</TableCell>
            <TableCell>
              <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
                <StatusIcon className='size-3.5' aria-hidden='true' />
                {statusLabel}
              </span>
            </TableCell>
            <TableCell className='text-muted-foreground text-xs'>
              {formatTimestampToDate(item.created_at)}
            </TableCell>
          </TableRow>
        )
      })}
    </>
  )
}

function ReferralInviteeRows({
  loading,
  data,
}: {
  loading: boolean
  data: ReferralPage<ReferralInviteeItem> | undefined
}) {
  const { t } = useTranslation()
  if (loading) {
    return (
      <TableRow>
        <TableCell colSpan={3}>
          <Loader2 className='text-muted-foreground mx-auto size-5 animate-spin' />
        </TableCell>
      </TableRow>
    )
  }
  if (!data?.items.length) {
    return (
      <TableRow>
        <TableCell
          colSpan={3}
          className='text-muted-foreground h-24 text-center'
        >
          {t('No invited users yet')}
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {data.items.map((item) => (
        <TableRow key={`${item.created_at}-${item.username}`}>
          <TableCell className='font-medium'>{item.username}</TableCell>
          <TableCell className='text-muted-foreground text-xs'>
            {formatTimestampToDate(item.created_at)}
          </TableCell>
          <TableCell>
            {item.qualified ? (
              <span className='text-xs text-emerald-600'>{t('Qualified')}</span>
            ) : (
              <span className='text-muted-foreground text-xs'>
                {t('Waiting for first top-up')}
              </span>
            )}
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}

export function Referrals() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { copyToClipboard } = useCopyToClipboard()
  const cachedUser = useAuthStore((state) => state.auth.user)
  const [transferOpen, setTransferOpen] = useState(false)

  const overviewQuery = useQuery({
    queryKey: ['referral', 'overview'],
    queryFn: async () => {
      const response = await getReferralOverview()
      queryError(response)
      return response.data as ReferralOverview
    },
  })
  const rewardsQuery = useQuery({
    queryKey: ['referral', 'rewards'],
    queryFn: async () => {
      const response = await getReferralRewards()
      queryError(response)
      return response.data ?? { page: 1, page_size: 50, total: 0, items: [] }
    },
  })
  const inviteesQuery = useQuery({
    queryKey: ['referral', 'invitees'],
    queryFn: async () => {
      const response = await getReferralInvitees()
      queryError(response)
      return response.data ?? { page: 1, page_size: 50, total: 0, items: [] }
    },
  })
  const selfQuery = useQuery<AuthUser | undefined>({
    queryKey: ['self'],
    queryFn: async () => {
      const response = await getSelf()
      queryError(response)
      return response.data as AuthUser | undefined
    },
    initialData: cachedUser ?? undefined,
  })
  const transferMutation = useMutation({
    mutationFn: transferReferralQuota,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Transfer failed'))
        return
      }
      toast.success(t('Transfer successful'))
      await queryClient.invalidateQueries({ queryKey: ['referral'] })
      await queryClient.invalidateQueries({ queryKey: ['self'] })
    },
    onError: (error: Error) =>
      toast.error(error.message || t('Transfer failed')),
  })

  const overview = overviewQuery.data
  const rewardRate = formatRewardRate(overview?.rules)
  const inviteUrl = useMemo(() => {
    if (!overview?.affiliate_code || typeof window === 'undefined') return ''
    return `${window.location.origin}/sign-up?aff=${encodeURIComponent(overview.affiliate_code)}`
  }, [overview?.affiliate_code])

  const shareInvite = async () => {
    if (!inviteUrl) return
    if (navigator.share) {
      try {
        await navigator.share({
          title: t('Benefit API referral'),
          text: t('Join Benefit API with my invite link.'),
          url: inviteUrl,
        })
        return
      } catch {
        return
      }
    }
    copyToClipboard(inviteUrl)
  }

  const loading = overviewQuery.isLoading
  const disabled = overview != null && !overview.enabled
  const rules = overview?.rules
  let qrContent: ReactNode
  if (loading) {
    qrContent = (
      <Loader2 className='text-muted-foreground size-6 animate-spin' />
    )
  } else if (inviteUrl) {
    qrContent = (
      <QRCodeSVG
        value={inviteUrl}
        size={108}
        includeMargin
        className='size-24 sm:size-28'
      />
    )
  } else {
    qrContent = <Share2 className='text-muted-foreground size-10' />
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Referral Center')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <Card className='border-primary/20 bg-primary/[0.06] overflow-visible'>
              <CardContent className='grid gap-5 p-4 sm:p-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
                <div className='min-w-0'>
                  <div className='text-primary flex items-center gap-2'>
                    <Gift className='size-5' aria-hidden='true' />
                    <span className='text-xs font-semibold tracking-[0.18em] uppercase'>
                      {t('Invite and earn')}
                    </span>
                  </div>
                  <h1 className='mt-2 text-2xl font-semibold tracking-tight sm:text-3xl'>
                    {t('Grow together with Benefit API')}
                  </h1>
                  <p className='text-muted-foreground mt-2 max-w-2xl text-sm leading-6'>
                    {t(
                      'Invite friends to try the API. When a new user completes a valid first top-up, both sides receive quota rewards.'
                    )}
                  </p>
                  {disabled && (
                    <p className='border-border/60 bg-background/60 text-muted-foreground mt-3 inline-flex rounded-lg border px-3 py-2 text-xs'>
                      {t(
                        'The referral program is currently paused by the administrator.'
                      )}
                    </p>
                  )}
                  {!overview?.compliance_ready && (
                    <p className='text-muted-foreground mt-3 text-xs'>
                      {t(
                        'Rewards and transfers will be available after payment compliance is confirmed.'
                      )}
                    </p>
                  )}
                  <div className='mt-4 flex flex-wrap items-center gap-2'>
                    <div className='border-border/60 bg-background/60 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2'>
                      <WalletCards
                        className='text-primary size-4 shrink-0'
                        aria-hidden='true'
                      />
                      <span className='text-muted-foreground text-xs'>
                        {t('Current balance')}
                      </span>
                      <span className='truncate text-sm font-semibold tabular-nums'>
                        {selfQuery.isLoading
                          ? t('Loading')
                          : formatQuota(selfQuery.data?.quota ?? 0)}
                      </span>
                    </div>
                    <Button
                      variant='outline'
                      className='min-h-11'
                      render={<Link to='/wallet' />}
                    >
                      <Plus aria-hidden='true' />
                      {t('Recharge')}
                    </Button>
                  </div>
                </div>
                <div className='border-primary/15 bg-background/70 flex size-28 items-center justify-center rounded-2xl border shadow-sm sm:size-32'>
                  {qrContent}
                </div>
              </CardContent>
            </Card>

            <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6'>
              <ReferralStat
                icon={UsersRound}
                label={t('Invites')}
                value={String(overview?.invite_count ?? 0)}
                tone='chart-1'
              />
              <ReferralStat
                icon={CheckCircle2}
                label={t('Qualified invites')}
                value={String(overview?.qualified_count ?? 0)}
                tone='chart-2'
              />
              <ReferralStat
                icon={Clock3}
                label={t('Pending rewards')}
                value={formatQuota(overview?.pending_quota ?? 0)}
                tone='chart-3'
              />
              <ReferralStat
                icon={WalletCards}
                label={t('Available rewards')}
                value={formatQuota(overview?.available_quota ?? 0)}
                tone='chart-4'
              />
              <ReferralStat
                icon={Percent}
                label={t('Reward rate')}
                value={rewardRate}
                tone='chart-2'
              />
              <ReferralStat
                icon={History}
                label={t('Total earned')}
                value={formatQuota(overview?.total_reward_quota ?? 0)}
                tone='chart-3'
              />
            </div>

            <Card>
              <CardHeader>
                <CardTitle>{t('Your invite code and link')}</CardTitle>
                <CardDescription>
                  {t(
                    'Share this link with friends. It opens the registration page with your invite code.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4 lg:grid-cols-[minmax(0,0.78fr)_minmax(0,1.22fr)]'>
                <div className='min-w-0 space-y-2'>
                  <label
                    htmlFor='referral-invite-code'
                    className='text-muted-foreground text-xs font-medium'
                  >
                    {t('Invite code')}
                  </label>
                  <div className='flex min-w-0 gap-2'>
                    <Input
                      id='referral-invite-code'
                      readOnly
                      value={overview?.affiliate_code ?? ''}
                      placeholder={t('Loading invite code')}
                      className='min-h-11 min-w-0 flex-1 font-mono text-sm'
                    />
                    <CopyButton
                      value={overview?.affiliate_code ?? ''}
                      size='lg'
                      variant='outline'
                      className={`min-h-11 ${!overview?.affiliate_code ? 'pointer-events-none opacity-50' : ''}`}
                      tooltip={t('Copy invite code')}
                      aria-label={t('Copy invite code')}
                    >
                      {t('Copy')}
                    </CopyButton>
                  </div>
                </div>
                <div className='min-w-0 space-y-2'>
                  <label
                    htmlFor='referral-invite-link'
                    className='text-muted-foreground text-xs font-medium'
                  >
                    {t('Invite link')}
                  </label>
                  <div className='flex min-w-0 flex-col gap-2 sm:flex-row'>
                    <Input
                      id='referral-invite-link'
                      readOnly
                      value={inviteUrl}
                      placeholder={t('Loading invite link')}
                      className='min-h-11 min-w-0 flex-1 font-mono text-xs'
                    />
                    <div className='flex flex-wrap gap-2'>
                      <CopyButton
                        value={inviteUrl}
                        size='lg'
                        variant='outline'
                        className={`min-h-11 ${!inviteUrl ? 'pointer-events-none opacity-50' : ''}`}
                        tooltip={t('Copy invite link')}
                        aria-label={t('Copy invite link')}
                      >
                        {t('Copy')}
                      </CopyButton>
                      <Button
                        variant='outline'
                        className='min-h-11'
                        onClick={shareInvite}
                        disabled={!inviteUrl}
                      >
                        <Share2 aria-hidden='true' />
                        {t('Share invite link')}
                      </Button>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>{t('How it works')}</CardTitle>
                <CardDescription>
                  {t('Invite in three simple steps.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4 border-t pt-4 sm:grid-cols-3'>
                <ReferralStep
                  number='01'
                  icon={Share2}
                  title={t('Share your invite code or link')}
                  description={t(
                    'Send your code or link to someone who needs API access.'
                  )}
                />
                <ReferralStep
                  number='02'
                  icon={UserPlus}
                  title={t('A friend signs up and tops up')}
                  description={t(
                    'Their first valid top-up activates the referral reward.'
                  )}
                />
                <ReferralStep
                  number='03'
                  icon={WalletCards}
                  title={t('Rewards become available')}
                  description={t(
                    'After the settlement delay, transfer available rewards to your balance.'
                  )}
                />
              </CardContent>
            </Card>

            {rules && (
              <Card>
                <CardHeader>
                  <CardTitle>{t('Referral rules')}</CardTitle>
                  <CardDescription>
                    {t(
                      'Rewards are quota only. They cannot be withdrawn as cash.'
                    )}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ReferralRulesList rules={rules} />
                </CardContent>
              </Card>
            )}

            <div className='grid gap-4 xl:grid-cols-[minmax(0,1.08fr)_minmax(0,0.92fr)]'>
              <Card>
                <CardHeader className='flex flex-row items-center justify-between gap-3'>
                  <div>
                    <CardTitle>{t('Reward history')}</CardTitle>
                    <CardDescription>
                      {t('Your inviter rewards and settlement status.')}
                    </CardDescription>
                  </div>
                  <Button
                    size='sm'
                    onClick={() => setTransferOpen(true)}
                    disabled={
                      !overview?.available_quota || !overview.compliance_ready
                    }
                  >
                    <WalletCards aria-hidden='true' />
                    {t('Transfer to Balance')}
                  </Button>
                </CardHeader>
                <CardContent>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Invitee')}</TableHead>
                        <TableHead>{t('Reward')}</TableHead>
                        <TableHead>{t('Status')}</TableHead>
                        <TableHead>{t('Date')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      <ReferralRewardRows
                        loading={rewardsQuery.isLoading}
                        data={rewardsQuery.data}
                      />
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>{t('Invited users')}</CardTitle>
                  <CardDescription>
                    {t('Only masked usernames are shown for privacy.')}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('User')}</TableHead>
                        <TableHead>{t('Joined')}</TableHead>
                        <TableHead>{t('Progress')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      <ReferralInviteeRows
                        loading={inviteesQuery.isLoading}
                        data={inviteesQuery.data}
                      />
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TransferDialog
        open={transferOpen}
        onOpenChange={setTransferOpen}
        onConfirm={async (quota) => {
          const response = await transferMutation.mutateAsync(quota)
          return response.success === true
        }}
        availableQuota={overview?.available_quota ?? 0}
        transferring={transferMutation.isPending}
      />
    </>
  )
}
