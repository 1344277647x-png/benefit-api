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
  defaultValues: { enabled: boolean; startAt: number; endAt: number }
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

          <div className='border-border/60 bg-muted/20 text-muted-foreground rounded-xl border p-4 text-sm leading-6'>
            <p>{t('Every ¥50 of eligible top-ups earns one draw.')}</p>
            <p>
              {t(
                'The 50th draw uses the jackpot pool. The ¥100 prize has a 0.001% chance in both pools.'
              )}
            </p>
            <p>
              {t(
                'Administrator credits, gifts, referrals, balance purchases and subscriptions do not count.'
              )}
            </p>
            <p>
              {t('Regular pool: ¥0.5 74.999%, ¥1 20%, ¥2 5%, ¥100 0.001%.')}
            </p>
            <p>
              {t('Jackpot pool: ¥5 79.999%, ¥10 15%, ¥50 5%, ¥100 0.001%.')}
            </p>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
