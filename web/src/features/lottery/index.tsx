import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Gift, History, Loader2, Sparkles, Trophy, Wallet } from 'lucide-react'
import { useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import { drawLottery, getLotteryHistory, getLotteryStatus } from './api'
import type { ApiResponse, LotteryStatus } from './types'
import {
  LOTTERY_WHEEL_REWARDS_CENTS,
  LOTTERY_WHEEL_STEP_DEGREES,
  nextWheelRotation,
} from './wheel-config'

const wheelColors = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
  'var(--primary)',
  'var(--accent-foreground)',
]

function requireData<T>(response: ApiResponse<T>): T {
  if (!response.success || !response.data) {
    throw new Error(response.message || 'Unable to load lottery data')
  }
  return response.data
}

function formatPrize(cents: number) {
  const amount = cents / 100
  return `¥${Number.isInteger(amount) ? amount.toFixed(0) : amount.toFixed(1)}`
}

function probabilityLabel(weight: number) {
  const percent = weight / 1000
  return `${percent
    .toFixed(3)
    .replace(/\.0+$/, '')
    .replace(/(\.\d*?)0+$/, '$1')}%`
}

function createRequestKey() {
  if (
    typeof globalThis.crypto !== 'undefined' &&
    typeof globalThis.crypto.randomUUID === 'function'
  ) {
    return globalThis.crypto.randomUUID()
  }
  return `lottery-${Date.now()}-${performance.now()}`
}

function LotteryWheel({ rotation }: { rotation: number }) {
  const { t } = useTranslation()
  const gradient = useMemo(() => {
    const stops = LOTTERY_WHEEL_REWARDS_CENTS.map((_, index) => {
      const start = index * LOTTERY_WHEEL_STEP_DEGREES
      const end = (index + 1) * LOTTERY_WHEEL_STEP_DEGREES
      return `${wheelColors[index]} ${start}deg ${end}deg`
    })
    return `conic-gradient(from -${LOTTERY_WHEEL_STEP_DEGREES / 2}deg, ${stops.join(', ')})`
  }, [])

  return (
    <div className='relative mx-auto aspect-square w-full max-w-[390px]'>
      <div className='border-background absolute top-[-2px] left-1/2 z-20 h-0 w-0 -translate-x-1/2 border-x-[14px] border-t-[24px] border-x-transparent drop-shadow-md' />
      <div
        className='border-border/70 shadow-primary/15 relative size-full rounded-full border-[10px] shadow-2xl transition-transform duration-[2800ms] ease-[cubic-bezier(.12,.65,.18,1)] motion-reduce:transition-none'
        style={{ background: gradient, transform: `rotate(${rotation}deg)` }}
        role='img'
        aria-label={t(
          'Prize wheel showing ¥0.5, ¥1, ¥2, ¥5, ¥10, ¥50 and ¥100'
        )}
      >
        {LOTTERY_WHEEL_REWARDS_CENTS.map((reward, index) => {
          const angle = index * LOTTERY_WHEEL_STEP_DEGREES
          const radians = (angle * Math.PI) / 180
          return (
            <span
              key={reward}
              className='absolute z-10 flex w-16 -translate-x-1/2 -translate-y-1/2 justify-center text-sm font-black text-white drop-shadow-md sm:text-base'
              style={{
                left: `${50 + Math.sin(radians) * 38}%`,
                top: `${50 - Math.cos(radians) * 38}%`,
              }}
            >
              {formatPrize(reward)}
            </span>
          )
        })}
        <div className='bg-background border-primary/30 absolute top-1/2 left-1/2 flex size-20 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border-4 shadow-lg'>
          <Sparkles className='text-primary size-8' aria-hidden='true' />
        </div>
      </div>
    </div>
  )
}

export function Lottery() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [rotation, setRotation] = useState(0)
  const [resultText, setResultText] = useState('')
  const currentIndex = useRef(0)

  const statusQuery = useQuery({
    queryKey: ['lottery', 'status'],
    queryFn: async () => requireData(await getLotteryStatus()),
  })
  const historyQuery = useQuery({
    queryKey: ['lottery', 'history'],
    queryFn: async () => requireData(await getLotteryHistory()),
  })
  const drawMutation = useMutation({
    mutationFn: () => drawLottery(createRequestKey()),
    onSuccess: async (response) => {
      const result = requireData(response)
      const next = nextWheelRotation(
        rotation,
        currentIndex.current,
        result.draw.reward_cents
      )
      currentIndex.current = next.index
      setRotation(next.rotation)
      setResultText(
        t('Congratulations! You received {{amount}} in API quota.', {
          amount: formatPrize(result.draw.reward_cents),
        })
      )
      queryClient.setQueryData<LotteryStatus>(
        ['lottery', 'status'],
        result.status
      )
      await queryClient.invalidateQueries({ queryKey: ['lottery', 'history'] })
      await queryClient.invalidateQueries({ queryKey: ['self'] })
      toast.success(
        t('Lottery reward credited: {{amount}}', {
          amount: formatPrize(result.draw.reward_cents),
        })
      )
    },
    onError: (error: Error) => toast.error(error.message || t('Draw failed')),
  })

  const status = statusQuery.data
  const activePool = status?.next_draw_is_jackpot
    ? status.jackpot_pool
    : status?.regular_pool
  const canDraw =
    status?.active === true &&
    (status.available_draws ?? 0) > 0 &&
    !drawMutation.isPending

  let historyRows: ReactNode
  if (historyQuery.isLoading) {
    historyRows = (
      <TableRow>
        <TableCell colSpan={4} className='h-20 text-center'>
          <Loader2 className='text-muted-foreground mx-auto size-5 animate-spin' />
        </TableCell>
      </TableRow>
    )
  } else if (historyQuery.data?.items.length) {
    historyRows = historyQuery.data.items.map((draw) => (
      <TableRow key={draw.id}>
        <TableCell>#{draw.draw_number}</TableCell>
        <TableCell>
          {draw.tier === 'jackpot' ? t('Jackpot') : t('Regular')}
        </TableCell>
        <TableCell className='font-semibold'>
          {formatPrize(draw.reward_cents)}
        </TableCell>
        <TableCell className='text-muted-foreground text-xs'>
          {formatTimestampToDate(draw.created_at)}
        </TableCell>
      </TableRow>
    ))
  } else {
    historyRows = (
      <TableRow>
        <TableCell
          colSpan={4}
          className='text-muted-foreground h-20 text-center'
        >
          {t('No draws yet')}
        </TableCell>
      </TableRow>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Lucky Draw')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto grid w-full max-w-7xl min-w-0 grid-cols-[minmax(0,1fr)] gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]'>
          <Card className='border-primary/20 min-w-0 overflow-hidden'>
            <CardHeader className='text-center'>
              <div className='text-primary mx-auto flex items-center gap-2 text-xs font-semibold tracking-[0.18em] uppercase'>
                <Gift className='size-4' aria-hidden='true' />
                {t('Recharge rewards')}
              </div>
              <CardTitle className='text-2xl sm:text-3xl'>
                {status?.next_draw_is_jackpot
                  ? t('Your next draw uses the jackpot pool')
                  : t('Every ¥50 recharged earns one draw')}
              </CardTitle>
              <CardDescription>
                {t(
                  'Only successful cash top-ups to your personal balance count. Spending balance on plans does not count again.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-5 p-4 sm:p-6'>
              <LotteryWheel rotation={rotation} />
              <p className='text-muted-foreground text-center text-xs'>
                {t(
                  'Wheel segments are visual only. Actual results follow the published probability table.'
                )}
              </p>
              {resultText && (
                <p className='bg-primary/10 text-primary rounded-xl px-4 py-3 text-center text-sm font-semibold'>
                  {resultText}
                </p>
              )}
              {status != null && !status.active && (
                <p className='border-border bg-muted/40 text-muted-foreground rounded-xl border px-4 py-3 text-center text-sm'>
                  {t('The lottery activity is currently paused.')}
                </p>
              )}
              {statusQuery.isError && (
                <p className='border-destructive/30 bg-destructive/10 text-destructive rounded-xl border px-4 py-3 text-center text-sm'>
                  {t('Unable to load lottery status. Please try again later.')}
                </p>
              )}
              <div className='flex flex-col justify-center gap-3 sm:flex-row'>
                <Button
                  size='lg'
                  className='min-h-12 min-w-40'
                  disabled={!canDraw}
                  onClick={() => drawMutation.mutate()}
                >
                  {drawMutation.isPending ? (
                    <Loader2 className='animate-spin' aria-hidden='true' />
                  ) : (
                    <Sparkles aria-hidden='true' />
                  )}
                  {t('Draw now')}
                </Button>
                <Button
                  size='lg'
                  variant='outline'
                  className='min-h-12'
                  render={<Link to='/wallet' />}
                >
                  <Wallet aria-hidden='true' />
                  {t('Recharge balance')}
                </Button>
              </div>
            </CardContent>
          </Card>

          <div className='min-w-0 space-y-5'>
            <div className='grid grid-cols-2 gap-3'>
              <Card>
                <CardContent className='p-4'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Available draws')}
                  </p>
                  <p className='mt-2 text-2xl font-semibold tabular-nums'>
                    {status?.available_draws ?? 0}
                  </p>
                </CardContent>
              </Card>
              <Card>
                <CardContent className='p-4'>
                  <p className='text-muted-foreground text-xs'>
                    {t('Until next draw')}
                  </p>
                  <p className='mt-2 text-2xl font-semibold tabular-nums'>
                    {formatPrize(status?.amount_to_next_draw_cents ?? 5000)}
                  </p>
                </CardContent>
              </Card>
            </div>

            <Card>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Trophy className='text-primary size-5' aria-hidden='true' />
                  {status?.next_draw_is_jackpot
                    ? t('Current jackpot pool')
                    : t('Current regular pool')}
                </CardTitle>
                <CardDescription>
                  {t('Every 50th personal draw uses the jackpot pool.')}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='grid grid-cols-2 gap-2'>
                  {(activePool ?? []).map((prize) => (
                    <div
                      key={`${prize.reward_cents}-${prize.weight}`}
                      className='border-border/60 bg-muted/25 rounded-lg border p-3'
                    >
                      <p className='font-semibold'>
                        {formatPrize(prize.reward_cents)}
                      </p>
                      <p className='text-muted-foreground text-xs tabular-nums'>
                        {probabilityLabel(prize.weight)}
                      </p>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <History className='size-5' aria-hidden='true' />
                  {t('Draw history')}
                </CardTitle>
                <CardDescription>
                  {t('Rewards are API quota and cannot be withdrawn as cash.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='overflow-x-auto'>
                <Table className='min-w-[460px]'>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Draw number')}</TableHead>
                      <TableHead>{t('Pool')}</TableHead>
                      <TableHead>{t('Reward')}</TableHead>
                      <TableHead>{t('Date')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>{historyRows}</TableBody>
                </Table>
                {historyQuery.isError && (
                  <p className='text-destructive py-3 text-center text-sm'>
                    {t('Unable to load draw history.')}
                  </p>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardContent className='grid grid-cols-2 gap-4 p-4 text-sm'>
                <div>
                  <p className='text-muted-foreground'>{t('Total draws')}</p>
                  <p className='font-semibold tabular-nums'>
                    {status?.total_draws ?? 0}
                  </p>
                </div>
                <div>
                  <p className='text-muted-foreground'>{t('Total rewards')}</p>
                  <p className='font-semibold tabular-nums'>
                    {formatQuota(status?.total_reward_quota ?? 0)}
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
