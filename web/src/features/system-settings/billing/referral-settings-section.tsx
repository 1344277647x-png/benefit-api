import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type Control, type Resolver } from 'react-hook-form'
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
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateReferralOptions } from '../hooks/use-update-option'

const schema = z.object({
  enabled: z.boolean(),
  minimumTopupAmount: z.coerce.number().min(0),
  rewardRatePercent: z.coerce.number().min(0).max(100),
  inviteeBonusAmount: z.coerce.number().min(0),
  perInviteeCapAmount: z.coerce.number().min(0),
  monthlyCapAmount: z.coerce.number().min(0),
  delayHours: z.coerce.number().int().min(0).max(720),
})

type Values = z.infer<typeof schema>

type ReferralSettingsSectionProps = {
  defaultValues: {
    enabled: boolean
    minimumTopupQuota: number
    rewardRateBasisPoints: number
    inviteeBonusQuota: number
    perInviteeCapQuota: number
    monthlyCapQuota: number
    settlementDelayHours: number
  }
  complianceConfirmed: boolean
}

export function ReferralSettingsSection({
  defaultValues,
  complianceConfirmed,
}: ReferralSettingsSectionProps) {
  const { t } = useTranslation()
  const updateReferralOptions = useUpdateReferralOptions()
  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues: {
      enabled: defaultValues.enabled,
      minimumTopupAmount: quotaUnitsToDollars(defaultValues.minimumTopupQuota),
      rewardRatePercent: defaultValues.rewardRateBasisPoints / 100,
      inviteeBonusAmount: quotaUnitsToDollars(defaultValues.inviteeBonusQuota),
      perInviteeCapAmount: quotaUnitsToDollars(
        defaultValues.perInviteeCapQuota
      ),
      monthlyCapAmount: quotaUnitsToDollars(defaultValues.monthlyCapQuota),
      delayHours: defaultValues.settlementDelayHours,
    },
  })

  const { isDirty, isSubmitting } = form.formState

  async function onSubmit(values: Values) {
    if (values.enabled && !complianceConfirmed) {
      toast.error(t('Confirm payment compliance before enabling referrals'))
      return
    }
    await updateReferralOptions.mutateAsync({
      enabled: values.enabled,
      minimum_topup_quota: parseQuotaFromDollars(values.minimumTopupAmount),
      reward_rate_basis_points: Math.round(values.rewardRatePercent * 100),
      invitee_bonus_quota: parseQuotaFromDollars(values.inviteeBonusAmount),
      per_invitee_cap_quota: parseQuotaFromDollars(values.perInviteeCapAmount),
      monthly_cap_quota: parseQuotaFromDollars(values.monthlyCapAmount),
      settlement_delay_hours: values.delayHours,
    })
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Referral Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateReferralOptions.isPending || isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save referral settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <div className='border-border/60 bg-muted/20 flex items-center justify-between gap-4 rounded-xl border p-4'>
                <div>
                  <FormLabel>{t('Enable referral center')}</FormLabel>
                  <FormDescription>
                    {complianceConfirmed
                      ? t(
                          'Reward users after a friend completes a valid first top-up.'
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
                      !complianceConfirmed ||
                      updateReferralOptions.isPending ||
                      isSubmitting
                    }
                  />
                </FormControl>
              </div>
            )}
          />

          <div className='grid gap-5 sm:grid-cols-2'>
            <AmountField
              name='minimumTopupAmount'
              label={t('Minimum qualifying top-up')}
              description={t(
                'A new user must reach this amount on their first successful top-up.'
              )}
              control={form.control}
            />
            <FormField
              control={form.control}
              name='rewardRatePercent'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Inviter reward rate (%)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={100}
                      step={0.5}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Percentage of the qualifying top-up credited to the inviter after the settlement delay.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <AmountField
              name='inviteeBonusAmount'
              label={t('Invitee bonus')}
              description={t(
                'Quota credited to the invited user immediately after the qualifying top-up.'
              )}
              control={form.control}
            />
            <AmountField
              name='perInviteeCapAmount'
              label={t('Per invitee cap')}
              description={t('Set to 0 for no per-user cap.')}
              control={form.control}
            />
            <AmountField
              name='monthlyCapAmount'
              label={t('Monthly reward cap')}
              description={t('Set to 0 for no monthly cap.')}
              control={form.control}
            />
            <FormField
              control={form.control}
              name='delayHours'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Settlement delay (hours)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={720}
                      step={1}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Pending rewards become transferable after this delay.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

function AmountField({
  name,
  label,
  description,
  control,
}: {
  name:
    | 'minimumTopupAmount'
    | 'inviteeBonusAmount'
    | 'perInviteeCapAmount'
    | 'monthlyCapAmount'
  label: string
  description: string
  control: Control<Values>
}) {
  const { t } = useTranslation()
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input type='number' min={0} step={0.01} {...field} />
          </FormControl>
          <FormDescription>
            {description}{' '}
            {t('Current quota value: {{amount}}.', {
              amount: formatQuota(
                parseQuotaFromDollars(Number(field.value) || 0)
              ),
            })}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
