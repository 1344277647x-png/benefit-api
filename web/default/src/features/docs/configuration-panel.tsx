/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  Check,
  Clipboard,
  Code2,
  Copy,
  KeyRound,
  PackageCheck,
  Server,
  Terminal,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { markOnboardingFlag, ONBOARDING_STORAGE_KEYS } from '@/lib/onboarding'

import {
  buildClientTemplate,
  compatiblePresets,
  type ClientPreset,
} from '../integrations/client-config'
import {
  getIntegrationModels,
  type IntegrationModel,
} from '../integrations/model-catalog'

type ConfigurationKey =
  | 'curl'
  | 'openai'
  | 'codex'
  | 'claude'
  | 'gemini'
  | 'cherry'
  | 'image'
  | 'video'
  | 'embeddings'
  | 'rerank'
type RequestMode = 'chat' | 'responses'

interface ConfigurationTemplate {
  key: ConfigurationKey
  title: string
  description: string
  language: string
  content: string
  endpoint: string
  install?: string
  path?: string
}

interface ConfigurationPanelProps {
  apiBaseUrl: string
}

function buildTemplates(
  apiBaseUrl: string,
  model: string,
  mode: RequestMode,
  selectedModel?: IntegrationModel
): ConfigurationTemplate[] {
  const candidates: ClientPreset[] = [
    'curl',
    'openai',
    'codex',
    'claude',
    'gemini',
    'cherry',
  ]
  const protocol = mode === 'responses' ? 'responses' : 'chat'
  const allowed = new Set(compatiblePresets(selectedModel))
  return candidates
    .filter((preset) => allowed.has(preset))
    .map((preset) => {
      const template = buildClientTemplate(
        preset,
        model,
        apiBaseUrl,
        selectedModel,
        { protocol }
      )
      return {
        key: preset,
        title: template.title,
        description: template.description,
        language: template.language,
        content: template.content,
        endpoint: template.endpoint,
        install: template.install,
        path: template.path,
      }
    })
}

function ConfigurationCodeBlock(props: {
  template: ConfigurationTemplate
  onCopy: (content: string) => Promise<void>
  copied: boolean
}) {
  const { t } = useTranslation()

  return (
    <div className='bg-muted/35 overflow-hidden rounded-xl border'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2.5'>
        <div className='flex min-w-0 items-center gap-2'>
          <Code2
            className='text-muted-foreground size-4 shrink-0'
            aria-hidden='true'
          />
          <span className='truncate text-sm font-medium'>
            {props.template.title}
          </span>
          <span className='text-muted-foreground rounded border px-1.5 py-0.5 font-mono text-[10px] uppercase'>
            {props.template.language}
          </span>
        </div>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-8 shrink-0 gap-1.5 px-2.5'
          onClick={() => props.onCopy(props.template.content)}
          aria-label={t('Copy configuration')}
        >
          {props.copied ? (
            <Check data-icon='inline-start' className='text-success' />
          ) : (
            <Copy data-icon='inline-start' />
          )}
          {props.copied ? t('Copied') : t('Copy')}
        </Button>
      </div>
      <div className='px-3 py-2'>
        <p className='text-muted-foreground mb-2 text-xs'>
          {t(props.template.description)}
        </p>
        {(props.template.install || props.template.path) && (
          <div className='text-muted-foreground mb-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px]'>
            {props.template.install && (
              <span>
                <span className='font-medium'>{t('Install')}:</span>{' '}
                <code className='text-foreground'>
                  {props.template.install}
                </code>
              </span>
            )}
            {props.template.path && (
              <span>
                <span className='font-medium'>{t('Config path')}:</span>{' '}
                <code className='text-foreground'>{props.template.path}</code>
              </span>
            )}
          </div>
        )}
        <pre className='text-foreground max-h-64 overflow-auto font-mono text-xs leading-relaxed break-words whitespace-pre-wrap'>
          <code>{props.template.content}</code>
        </pre>
      </div>
    </div>
  )
}

export function ConfigurationPanel(props: ConfigurationPanelProps) {
  const { t } = useTranslation()
  const [activeKey, setActiveKey] = useState<ConfigurationKey>('curl')
  const [mode, setMode] = useState<RequestMode>('chat')
  const [model, setModel] = useState('YOUR_MODEL')
  const modelsQuery = useQuery({
    queryKey: ['integration-models'],
    queryFn: getIntegrationModels,
    staleTime: 60 * 1000,
    retry: 1,
  })
  const availableModels = useMemo(
    () => modelsQuery.data ?? [],
    [modelsQuery.data]
  )
  const selectedModel: IntegrationModel | undefined = availableModels.find(
    (item) => item.modelName === model
  )
  useEffect(() => {
    if (model === 'YOUR_MODEL' && availableModels[0]) {
      setModel(availableModels[0].modelName)
    }
  }, [availableModels, model])
  const effectiveMode: RequestMode = useMemo(() => {
    if (!selectedModel) return mode
    const hasChat = selectedModel.protocols.includes('openai-chat')
    const hasResponses = selectedModel.protocols.includes('openai-responses')
    if (mode === 'chat' && !hasChat && hasResponses) return 'responses'
    if (mode === 'responses' && !hasResponses && hasChat) return 'chat'
    return mode
  }, [mode, selectedModel])
  useEffect(() => {
    if (effectiveMode !== mode) setMode(effectiveMode)
  }, [effectiveMode, mode])
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const templates = useMemo(
    () => buildTemplates(props.apiBaseUrl, model, effectiveMode, selectedModel),
    [effectiveMode, model, props.apiBaseUrl, selectedModel]
  )
  const activeTemplate =
    templates.find((template) => template.key === activeKey) ?? templates[0]

  const handleCopy = async (content: string) => {
    const copied = await copyToClipboard(content)
    if (copied) {
      markOnboardingFlag(ONBOARDING_STORAGE_KEYS.configurationCopied)
    }
  }

  const handleCopyApiUrl = async () => {
    const copied = await copyToClipboard(props.apiBaseUrl)
    if (copied) {
      markOnboardingFlag(ONBOARDING_STORAGE_KEYS.apiUrlCopied)
    }
  }

  return (
    <Card className='mb-6 overflow-hidden'>
      <CardHeader className='from-primary/8 to-info/8 gap-2 border-b bg-linear-to-br via-transparent'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex min-w-0 items-start gap-3'>
            <span className='bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-xl'>
              <Terminal className='size-5' aria-hidden='true' />
            </span>
            <div className='min-w-0'>
              <CardTitle>{t('Connect your first client')}</CardTitle>
              <CardDescription className='mt-1 max-w-2xl'>
                {t(
                  'Use the current API address in your client. Replace the placeholder key and model before sending a request.'
                )}
              </CardDescription>
            </div>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='shrink-0 gap-1.5'
            onClick={handleCopyApiUrl}
            aria-label={t('Copy API URL')}
          >
            {copiedText === props.apiBaseUrl ? (
              <Check data-icon='inline-start' className='text-success' />
            ) : (
              <Clipboard data-icon='inline-start' />
            )}
            {t('Copy API URL')}
          </Button>
        </div>
        <div className='bg-background/75 mt-2 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2'>
          <span className='text-muted-foreground shrink-0 text-xs'>
            {t('API base URL')}
          </span>
          <code
            className='min-w-0 truncate font-mono text-xs sm:text-sm'
            title={props.apiBaseUrl}
          >
            {props.apiBaseUrl}
          </code>
        </div>
      </CardHeader>
      <CardContent className='space-y-4 pt-0'>
        <div className='grid gap-3 border-b py-4 sm:grid-cols-3'>
          <div className='flex items-start gap-2'>
            <span className='bg-primary text-primary-foreground flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
              1
            </span>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>{t('Create an API key')}</p>
              <Link
                to='/keys'
                className='text-muted-foreground hover:text-foreground mt-0.5 inline-flex items-center gap-1 text-xs underline-offset-4 hover:underline'
              >
                <KeyRound className='size-3.5' aria-hidden='true' />
                {t('Open key management')}
              </Link>
            </div>
          </div>
          <div className='flex items-start gap-2'>
            <span className='bg-muted text-muted-foreground flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
              2
            </span>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>{t('Choose a protocol')}</p>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t('Match the protocol to your client.')}
              </p>
            </div>
          </div>
          <div className='flex items-start gap-2'>
            <span className='bg-muted text-muted-foreground flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
              3
            </span>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>{t('Copy and run')}</p>
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {t('Replace the placeholders, then send your first request.')}
              </p>
            </div>
          </div>
        </div>

        <div className='grid gap-3 md:grid-cols-[minmax(0,1.2fr)_minmax(0,0.8fr)]'>
          <div className='space-y-2'>
            <div className='flex items-center gap-2'>
              <Server
                className='text-muted-foreground size-4'
                aria-hidden='true'
              />
              <p className='text-sm font-medium'>{t('Request protocol')}</p>
            </div>
            <div
              className='bg-muted/40 grid grid-cols-2 gap-1 rounded-lg border p-1'
              role='tablist'
              aria-label={t('Request protocol')}
            >
              {(['chat', 'responses'] as const).map((item) => (
                <Button
                  key={item}
                  type='button'
                  variant={effectiveMode === item ? 'default' : 'ghost'}
                  size='sm'
                  className='w-full'
                  role='tab'
                  aria-selected={effectiveMode === item}
                  disabled={
                    selectedModel !== undefined &&
                    !selectedModel.protocols.includes(
                      item === 'chat' ? 'openai-chat' : 'openai-responses'
                    )
                  }
                  onClick={() => {
                    setMode(item)
                    if (item === 'responses') {
                      setActiveKey('codex')
                    } else if (activeKey === 'codex') {
                      setActiveKey('curl')
                    }
                  }}
                >
                  {item === 'chat' ? t('Chat Completions') : t('Responses')}
                </Button>
              ))}
            </div>
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {effectiveMode === 'responses'
                ? t('Use Responses for Codex and modern reasoning clients.')
                : t(
                    'Use Chat Completions for OpenAI-compatible SDKs and clients.'
                  )}
            </p>
          </div>
          <div className='space-y-2'>
            <label htmlFor='docs-model' className='text-sm font-medium'>
              {t('Model identifier')}
            </label>
            <Input
              id='docs-model'
              list='docs-model-list'
              value={model}
              onChange={(event) => setModel(event.target.value || 'YOUR_MODEL')}
              placeholder='gpt-5.4'
              className='h-9 font-mono text-xs'
            />
            <datalist id='docs-model-list'>
              {availableModels.map((item) => (
                <option key={item.modelName} value={item.modelName} />
              ))}
            </datalist>
            <p className='text-muted-foreground text-xs leading-relaxed'>
              {selectedModel
                ? t('Model options are loaded from this Benefit API instance.')
                : t('Use the exact model ID shown in Model Square.')}
            </p>
            {modelsQuery.isFetching && (
              <p className='text-muted-foreground text-[11px]'>
                {t('Loading model capabilities...')}
              </p>
            )}
          </div>
        </div>

        <div
          className='grid grid-cols-2 gap-1.5 sm:flex sm:flex-wrap sm:items-center sm:gap-2'
          role='tablist'
          aria-label={t('Configuration template')}
        >
          {templates.map((template) => (
            <Button
              key={template.key}
              type='button'
              variant={activeKey === template.key ? 'default' : 'outline'}
              size='sm'
              className='w-full shrink-0 sm:w-auto'
              role='tab'
              aria-selected={activeKey === template.key}
              onClick={() => {
                setActiveKey(template.key)
                if (template.key === 'codex') setMode('responses')
              }}
            >
              {template.title}
            </Button>
          ))}
        </div>
        {activeTemplate ? (
          <ConfigurationCodeBlock
            template={activeTemplate}
            onCopy={handleCopy}
            copied={copiedText === activeTemplate.content}
          />
        ) : (
          <p className='text-muted-foreground rounded-lg border border-dashed px-3 py-4 text-xs'>
            {t(
              'This model exposes a non-chat endpoint. Open Skill Center for its complete protocol template.'
            )}
          </p>
        )}
        {activeKey === 'claude' &&
          selectedModel &&
          !selectedModel.protocols.includes('anthropic') && (
            <p className='text-warning-foreground rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs'>
              {t(
                'This model does not advertise the Claude Messages endpoint. Choose a Claude-compatible model before using this template.'
              )}
            </p>
          )}
        {activeTemplate && (
          <div className='bg-muted/30 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2'>
            <span className='text-muted-foreground shrink-0 text-xs'>
              {t('Request endpoint')}
            </span>
            <code
              className='min-w-0 flex-1 truncate font-mono text-xs'
              title={activeTemplate.endpoint}
            >
              {activeTemplate.endpoint}
            </code>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='h-8 shrink-0 gap-1.5 px-2.5'
              onClick={() => copyToClipboard(activeTemplate.endpoint)}
              aria-label={t('Copy endpoint')}
            >
              {copiedText === activeTemplate.endpoint ? (
                <Check data-icon='inline-start' className='text-success' />
              ) : (
                <Clipboard data-icon='inline-start' />
              )}
              {t('Copy')}
            </Button>
          </div>
        )}
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
          <span className='inline-flex items-center gap-1.5'>
            <PackageCheck className='size-3.5' aria-hidden='true' />
            {t('Your API key stays in your client and is never stored here.')}
          </span>
          <Link
            to='/pricing'
            className='text-foreground inline-flex items-center gap-1 underline-offset-4 hover:underline'
          >
            {t('Choose a model')}
            <ArrowRight className='size-3' aria-hidden='true' />
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}
