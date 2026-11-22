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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { buildClientTemplate } from '@/features/integrations/client-config'
import { getUserModels } from '@/lib/api'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { getApiBaseUrl, toClientApiKey } from '../../lib/server-address'

const CLIENT_PRESETS = [
  { key: 'codex', label: 'Codex CLI' },
  { key: 'claude', label: 'Claude Code' },
  { key: 'cherry', label: 'Cherry Studio' },
  { key: 'cursor', label: 'Cursor' },
] as const

type ClientConfigPreset = (typeof CLIENT_PRESETS)[number]['key']

const DEFAULT_PRESET: ClientConfigPreset = 'codex'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Real token key resolved on demand; never persisted by this dialog. */
  resolveKey: () => Promise<string | null>
}

/**
 * Builds Codex / Claude Code / Cherry Studio configuration for one key by
 * reusing the shared integration templates. The preview masks the key; the
 * real key is only inserted into the clipboard text on explicit copy.
 */
export function ClientConfigDialog(props: Props) {
  const { t } = useTranslation()
  const [preset, setPreset] = useState<ClientConfigPreset>(DEFAULT_PRESET)
  const [model, setModel] = useState('')

  const { data: modelsData } = useQuery({
    queryKey: ['user-models-ccswitch'],
    queryFn: getUserModels,
    enabled: props.open,
    staleTime: 5 * 60 * 1000,
  })

  const modelOptions = useMemo(() => {
    const items = modelsData?.data ?? []
    return items.map((item) => ({ value: item, label: item }))
  }, [modelsData?.data])

  const template = useMemo(
    () => buildClientTemplate(preset, model, getApiBaseUrl()),
    [preset, model]
  )

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      // Reset on close so the next key starts from a clean form.
      setPreset(DEFAULT_PRESET)
      setModel('')
    }
    props.onOpenChange(open)
  }

  const handleCopy = async () => {
    if (!model.trim()) {
      toast.warning(t('Please select a primary model'))
      return
    }
    const key = await props.resolveKey()
    if (!key) return
    const ok = await copyToClipboard(
      buildClientTemplate(preset, model, getApiBaseUrl(), undefined, {
        apiKey: toClientApiKey(key),
      }).content
    )
    if (ok) {
      toast.success(t('Copied'))
    } else {
      toast.error(t('Copy failed'))
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleOpenChange}
      title={t('Client configuration')}
      description={t('Copy a ready-to-use client configuration for this key.')}
      contentClassName='sm:max-w-xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleCopy}>{t('Copy configuration')}</Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Client')}</Label>
          <RadioGroup
            value={preset}
            onValueChange={(value) => setPreset(value as ClientConfigPreset)}
            className='flex flex-wrap gap-4'
          >
            {CLIENT_PRESETS.map((item) => (
              <div key={item.key} className='flex items-center gap-2'>
                <RadioGroupItem
                  value={item.key}
                  id={`client-config-${item.key}`}
                />
                <Label
                  htmlFor={`client-config-${item.key}`}
                  className='cursor-pointer'
                >
                  {item.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='client-config-model'>
            {t('Primary Model')}
            <span className='text-destructive ml-0.5'>*</span>
          </Label>
          <ComboboxInput
            allowCustomValue
            id='client-config-model'
            options={modelOptions}
            value={model}
            onValueChange={setModel}
            placeholder={t('Select or enter model name')}
            emptyText={t('No models found')}
          />
        </div>

        {(template.install || template.path) && (
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
            {template.install && (
              <span>
                <span className='font-medium'>{t('Install')}:</span>{' '}
                <code className='text-foreground break-all'>
                  {template.install}
                </code>
              </span>
            )}
            {template.path && (
              <span>
                <span className='font-medium'>{t('Config path')}:</span>{' '}
                <code className='text-foreground break-all'>
                  {template.path}
                </code>
              </span>
            )}
          </div>
        )}

        <div className='space-y-2'>
          <Label>{t('Configuration preview')}</Label>
          <pre
            className='bg-muted/40 max-h-64 overflow-auto rounded-lg border p-3 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap'
            aria-label={t('Configuration preview')}
          >
            <code>{template.content}</code>
          </pre>
          <p className='text-muted-foreground text-xs'>
            {t(
              'The key is masked in this preview and filled in only when you copy. Keep the copied configuration private.'
            )}
          </p>
        </div>
      </div>
    </Dialog>
  )
}
