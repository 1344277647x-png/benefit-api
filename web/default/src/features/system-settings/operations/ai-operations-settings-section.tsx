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
import { useEffect, useMemo, useRef } from 'react'
import {
  useForm,
  type Control,
  type FieldPath,
  type FieldValues,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const creationSchema = z.object({
  creation_setting: z.object({
    enabled: z.boolean(),
    retention_days: z.coerce.number().int().min(1).max(30),
    max_image_mb: z.coerce.number().int().min(1).max(20),
    max_video_mb: z.coerce.number().int().min(1).max(500),
    max_user_storage_mb: z.coerce.number().int().min(1).max(1024),
    max_system_storage_mb: z.coerce.number().int().min(1).max(10240),
    max_pending_video_jobs: z.coerce.number().int().min(1).max(2),
  }),
})

type CreationFormInput = z.input<typeof creationSchema>
type CreationFormValues = z.output<typeof creationSchema>

type FlatCreationDefaults = {
  'creation_setting.enabled': boolean
  'creation_setting.retention_days': number
  'creation_setting.max_image_mb': number
  'creation_setting.max_video_mb': number
  'creation_setting.max_user_storage_mb': number
  'creation_setting.max_system_storage_mb': number
  'creation_setting.max_pending_video_jobs': number
}

function creationFormDefaults(
  defaults: FlatCreationDefaults
): CreationFormInput {
  return {
    creation_setting: {
      enabled: defaults['creation_setting.enabled'],
      retention_days: defaults['creation_setting.retention_days'],
      max_image_mb: defaults['creation_setting.max_image_mb'],
      max_video_mb: defaults['creation_setting.max_video_mb'],
      max_user_storage_mb: defaults['creation_setting.max_user_storage_mb'],
      max_system_storage_mb: defaults['creation_setting.max_system_storage_mb'],
      max_pending_video_jobs:
        defaults['creation_setting.max_pending_video_jobs'],
    },
  }
}

function normalizeCreationValues(
  values: CreationFormValues
): FlatCreationDefaults {
  return {
    'creation_setting.enabled': values.creation_setting.enabled,
    'creation_setting.retention_days': values.creation_setting.retention_days,
    'creation_setting.max_image_mb': values.creation_setting.max_image_mb,
    'creation_setting.max_video_mb': values.creation_setting.max_video_mb,
    'creation_setting.max_user_storage_mb':
      values.creation_setting.max_user_storage_mb,
    'creation_setting.max_system_storage_mb':
      values.creation_setting.max_system_storage_mb,
    'creation_setting.max_pending_video_jobs':
      values.creation_setting.max_pending_video_jobs,
  }
}

type CreationSettingsProps = { defaultValues: FlatCreationDefaults }

export function CreationSettingsSection({
  defaultValues,
}: CreationSettingsProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => creationFormDefaults(defaultValues),
    [defaultValues]
  )
  const baselineRef = useRef(defaultValues)
  const baselineSerializedRef = useRef(JSON.stringify(defaultValues))
  const form = useForm<CreationFormInput, unknown, CreationFormValues>({
    resolver: zodResolver(creationSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    const serialized = JSON.stringify(defaultValues)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = defaultValues
    baselineSerializedRef.current = serialized
    form.reset(creationFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: CreationFormValues) => {
    const normalized = normalizeCreationValues(values)
    const changedKeys = (
      Object.keys(normalized) as Array<keyof FlatCreationDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])
    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }
    for (const key of changedKeys) {
      await updateOption.mutateAsync({ key, value: normalized[key] })
    }
    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
    form.reset(creationFormDefaults(normalized))
  }

  return (
    <SettingsSection title={t('AI Creation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='creation_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable AI creation center')}</FormLabel>
                  <FormDescription>
                    {t('Allow signed-in users to generate images and videos.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <NumberField
            control={form.control}
            name='creation_setting.retention_days'
            label={t('Result retention (days)')}
            min={1}
            max={30}
          />
          <NumberField
            control={form.control}
            name='creation_setting.max_image_mb'
            label={t('Maximum image size (MB)')}
            min={1}
            max={20}
          />
          <NumberField
            control={form.control}
            name='creation_setting.max_video_mb'
            label={t('Maximum video size (MB)')}
            min={1}
            max={500}
          />
          <NumberField
            control={form.control}
            name='creation_setting.max_user_storage_mb'
            label={t('Per-user storage limit (MB)')}
            min={1}
            max={1024}
          />
          <NumberField
            control={form.control}
            name='creation_setting.max_system_storage_mb'
            label={t('Site storage limit (MB)')}
            min={1}
            max={10240}
          />
          <NumberField
            control={form.control}
            name='creation_setting.max_pending_video_jobs'
            label={t('Concurrent video jobs per user')}
            min={1}
            max={2}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

const healthSchema = z.object({
  channel_health_setting: z.object({
    enabled: z.boolean(),
    refresh_interval_seconds: z.coerce.number().int().min(5).max(300),
    window_minutes: z.coerce.number().int().min(1).max(60),
    delayed_threshold_ms: z.coerce.number().int().min(100).max(3600000),
    failure_streak_threshold: z.coerce.number().int().min(1).max(100),
    minimum_samples: z.coerce.number().int().min(1).max(1000),
    unavailable_error_rate_percent: z.coerce.number().int().min(1).max(100),
    stale_after_minutes: z.coerce.number().int().min(1).max(10080),
    retention_days: z.coerce.number().int().min(1).max(365),
  }),
})

type HealthFormInput = z.input<typeof healthSchema>
type HealthFormValues = z.output<typeof healthSchema>

type FlatHealthDefaults = {
  'channel_health_setting.enabled': boolean
  'channel_health_setting.refresh_interval_seconds': number
  'channel_health_setting.window_minutes': number
  'channel_health_setting.delayed_threshold_ms': number
  'channel_health_setting.failure_streak_threshold': number
  'channel_health_setting.minimum_samples': number
  'channel_health_setting.unavailable_error_rate_percent': number
  'channel_health_setting.stale_after_minutes': number
  'channel_health_setting.retention_days': number
}

function healthFormDefaults(defaults: FlatHealthDefaults): HealthFormInput {
  return {
    channel_health_setting: {
      enabled: defaults['channel_health_setting.enabled'],
      refresh_interval_seconds:
        defaults['channel_health_setting.refresh_interval_seconds'],
      window_minutes: defaults['channel_health_setting.window_minutes'],
      delayed_threshold_ms:
        defaults['channel_health_setting.delayed_threshold_ms'],
      failure_streak_threshold:
        defaults['channel_health_setting.failure_streak_threshold'],
      minimum_samples: defaults['channel_health_setting.minimum_samples'],
      unavailable_error_rate_percent:
        defaults['channel_health_setting.unavailable_error_rate_percent'],
      stale_after_minutes:
        defaults['channel_health_setting.stale_after_minutes'],
      retention_days: defaults['channel_health_setting.retention_days'],
    },
  }
}

function normalizeHealthValues(values: HealthFormValues): FlatHealthDefaults {
  return {
    'channel_health_setting.enabled': values.channel_health_setting.enabled,
    'channel_health_setting.refresh_interval_seconds':
      values.channel_health_setting.refresh_interval_seconds,
    'channel_health_setting.window_minutes':
      values.channel_health_setting.window_minutes,
    'channel_health_setting.delayed_threshold_ms':
      values.channel_health_setting.delayed_threshold_ms,
    'channel_health_setting.failure_streak_threshold':
      values.channel_health_setting.failure_streak_threshold,
    'channel_health_setting.minimum_samples':
      values.channel_health_setting.minimum_samples,
    'channel_health_setting.unavailable_error_rate_percent':
      values.channel_health_setting.unavailable_error_rate_percent,
    'channel_health_setting.stale_after_minutes':
      values.channel_health_setting.stale_after_minutes,
    'channel_health_setting.retention_days':
      values.channel_health_setting.retention_days,
  }
}

type HealthSettingsProps = { defaultValues: FlatHealthDefaults }

export function ChannelHealthSettingsSection({
  defaultValues,
}: HealthSettingsProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => healthFormDefaults(defaultValues),
    [defaultValues]
  )
  const baselineRef = useRef(defaultValues)
  const baselineSerializedRef = useRef(JSON.stringify(defaultValues))
  const form = useForm<HealthFormInput, unknown, HealthFormValues>({
    resolver: zodResolver(healthSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    const serialized = JSON.stringify(defaultValues)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = defaultValues
    baselineSerializedRef.current = serialized
    form.reset(healthFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: HealthFormValues) => {
    const normalized = normalizeHealthValues(values)
    const changedKeys = (
      Object.keys(normalized) as Array<keyof FlatHealthDefaults>
    ).filter((key) => normalized[key] !== baselineRef.current[key])
    if (changedKeys.length === 0) {
      toast.info(t('No changes to save'))
      return
    }
    for (const key of changedKeys) {
      await updateOption.mutateAsync({ key, value: normalized[key] })
    }
    baselineRef.current = normalized
    baselineSerializedRef.current = JSON.stringify(normalized)
    form.reset(healthFormDefaults(normalized))
  }

  return (
    <SettingsSection title={t('Channel health')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='channel_health_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable channel health tracking')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Aggregate real traffic and channel tests into health states.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.refresh_interval_seconds'
            label={t('Health refresh interval (seconds)')}
            description={t(
              'Controls how often health pages refresh. Data comes from real requests and manual channel tests; no automatic paid probe is sent.'
            )}
            min={5}
            max={300}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.window_minutes'
            label={t('Health window (minutes)')}
            min={1}
            max={60}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.delayed_threshold_ms'
            label={t('Delayed threshold (ms)')}
            min={100}
            max={3600000}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.failure_streak_threshold'
            label={t('Failure streak threshold')}
            min={1}
            max={100}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.minimum_samples'
            label={t('Minimum samples')}
            min={1}
            max={1000}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.unavailable_error_rate_percent'
            label={t('Unavailable error rate (%)')}
            min={1}
            max={100}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.stale_after_minutes'
            label={t('Stale after (minutes)')}
            min={1}
            max={10080}
          />
          <NumberField
            control={form.control}
            name='channel_health_setting.retention_days'
            label={t('Health history retention (days)')}
            min={1}
            max={365}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

function NumberField<
  TFieldValues extends FieldValues,
  TName extends FieldPath<TFieldValues>,
>({
  control,
  name,
  label,
  description,
  min,
  max,
}: {
  control: Control<TFieldValues>
  name: TName
  label: string
  description?: string
  min: number
  max: number
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              type='number'
              min={min}
              max={max}
              step={1}
              {...safeNumberFieldProps(field)}
            />
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
