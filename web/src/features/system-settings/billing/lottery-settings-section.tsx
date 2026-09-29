import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateLotteryOptions } from '../hooks/use-update-option'

const schema = z
  .object({
    enabled: z.boolean(),
    startAt: z.string(),
    endAt: z.string(),
    regularWeights: z.array(z.coerce.number().min(0).max(100)).length(4),
    jackpotWeights: z.array(z.coerce.number().min(0).max(100)).length(4),
  })
  .refine(
    (values) => {
      if (!values.startAt || !values.endAt) return true
      return (
        new Date(values.endAt).getTime() > new Date(values.startAt).getTime()
      )
    },
    { path: ['endAt'], message: 'End time must be later than start time' }
  )
  .refine(
    (values) =>
      Math.abs(
        values.regularWeights.reduce((sum, value) => sum + value, 0) - 100
      ) < 0.0001,
    {
      path: ['regularWeights'],
      message: 'Probabilities must total 100.000%',
    }
  )
  .refine(
    (values) =>
      Math.abs(
        values.jackpotWeights.reduce((sum, value) => sum + value, 0) - 100
      ) < 0.0001,
    {
      path: ['jackpotWeights'],
      message: 'Probabilities must total 100.000%',
    }
  )

type Values = z.infer<typeof schema>

function unixToLocalInput(timestamp: number) {
  if (!timestamp) return ''
  const date = new Date(timestamp * 1000)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function localInputToUnix(value: string) {
  if (!value) return 0
  return Math.floor(new Date(value).getTime() / 1000)
}

type LotterySettingsSectionProps = {
  defaultValues: {
    enabled: boolean
    startAt: number
    endAt: number
    regularWeights: number[]
    jackpotWeights: number[]
  }
  complianceConfirmed: boolean
}

export function LotterySettingsSection(props: LotterySettingsSectionProps) {
  const { t } = useTranslation()
  const updateLotteryOptions = useUpdateLotteryOptions()
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: props.defaultValues.enabled,
      startAt: unixToLocalInput(props.defaultValues.startAt),
      endAt: unixToLocalInput(props.defaultValues.endAt),
      regularWeights: props.defaultValues.regularWeights.map(
        (weight) => weight / 1000
      ),
      jackpotWeights: props.defaultValues.jackpotWeights.map(
        (weight) => weight / 1000
      ),
    },
  })
  const { isDirty, isSubmitting } = form.formState

  async function onSubmit(values: Values) {
    if (values.enabled && !props.complianceConfirmed) {
      toast.error(t('Confirm payment compliance before enabling the lottery'))
      return
    }
    await updateLotteryOptions.mutateAsync({
      enabled: values.enabled,
      start_at: localInputToUnix(values.startAt),
      end_at: localInputToUnix(values.endAt),
      regular_weights: values.regularWeights.map((weight) =>
        Math.round(weight * 1000)
      ),
      jackpot_weights: values.jackpotWeights.map((weight) =>
        Math.round(weight * 1000)
      ),
    })
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Lottery Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateLotteryOptions.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save lottery settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <div className='border-border/60 bg-muted/20 flex items-center justify-between gap-4 rounded-xl border p-4'>
                <div>
                  <FormLabel>{t('Enable recharge lottery')}</FormLabel>
                  <FormDescription>
                    {props.complianceConfirmed
                      ? t(
                          'Only successful cash top-ups to personal balance earn draws.'
                        )
                      : t(
                          'Payment compliance must be confirmed before this feature can be enabled.'
                        )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={
                      !props.complianceConfirmed ||
                      updateLotteryOptions.isPending ||
                      isSubmitting
                    }
                  />
                </FormControl>
              </div>
            )}
          />

          <div className='grid gap-5 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='startAt'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Activity start time')}</FormLabel>
                  <FormControl>
                    <Input type='datetime-local' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Leave empty to start immediately after enabling.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='endAt'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Activity end time')}</FormLabel>
                  <FormControl>
                    <Input type='datetime-local' {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Leave empty for no scheduled end time.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {[
            {
              name: 'regularWeights' as const,
              title: 'Regular draw probabilities',
              prizes: ['¥0.5', '¥1', '¥2', '¥100'],
            },
            {
              name: 'jackpotWeights' as const,
              title: 'Every 50th draw probabilities',
              prizes: ['¥5', '¥10', '¥50', '¥100'],
            },
          ].map((pool) => (
            <div
              key={pool.name}
              className='border-border/60 rounded-xl border p-4'
            >
              <FormLabel>{t(pool.title)}</FormLabel>
              <div className='mt-3 grid gap-3 sm:grid-cols-4'>
                {pool.prizes.map((prize, index) => (
                  <FormField
                    key={prize}
                    control={form.control}
                    name={`${pool.name}.${index}`}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{prize}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min='0'
                            max='100'
                            step='0.001'
                            {...field}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                ))}
              </div>
              <FormMessage>
                {form.formState.errors[pool.name]?.root?.message ??
                  form.formState.errors[pool.name]?.message}
              </FormMessage>
            </div>
          ))}

          <div className='border-border/60 bg-muted/20 text-muted-foreground rounded-xl border p-4 text-sm leading-6'>
            <p>{t('Every ¥50 of eligible top-ups earns one draw.')}</p>
            <p>{t('The 50th draw uses the jackpot pool.')}</p>
            <p>
              {t(
                'Administrator credits, gifts, referrals, balance purchases and subscriptions do not count.'
              )}
            </p>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
