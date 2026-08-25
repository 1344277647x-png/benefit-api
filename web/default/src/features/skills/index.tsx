/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  Check,
  Clipboard,
  Code2,
  Download,
  ExternalLink,
  FileCode2,
  Layers3,
  RefreshCw,
  Search,
  Sparkles,
  Terminal,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
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

import {
  buildClientTemplate,
  compatiblePresets,
  type ClientPreset,
} from '../integrations/client-config'
import {
  getIntegrationModels,
  protocolLabel,
  type IntegrationModel,
} from '../integrations/model-catalog'

const presetMeta: Array<{
  preset: ClientPreset
  icon: typeof Terminal
  title: string
  description: string
  protocol: string
}> = [
  {
    preset: 'codex',
    icon: Terminal,
    title: 'Codex CLI',
    description:
      'Responses API configuration with a safe environment variable placeholder.',
    protocol: 'openai-responses',
  },
  {
    preset: 'claude',
    icon: Sparkles,
    title: 'Claude Code',
    description:
      'Anthropic Messages endpoint and Claude Code environment variables.',
    protocol: 'anthropic',
  },
  {
    preset: 'gemini',
    icon: Sparkles,
    title: 'Gemini',
    description:
      'Native Gemini generateContent configuration, including image output models.',
    protocol: 'gemini',
  },
  {
    preset: 'openai',
    icon: Code2,
    title: 'OpenAI SDK',
    description:
      'Python SDK template that follows the selected model endpoint.',
    protocol: 'openai-chat',
  },
  {
    preset: 'cherry',
    icon: Layers3,
    title: 'Cherry Studio',
    description: 'OpenAI-compatible provider fields for desktop clients.',
    protocol: 'openai-chat',
  },
  {
    preset: 'curl',
    icon: FileCode2,
    title: 'cURL / API',
    description: 'A minimal request you can run from any terminal or script.',
    protocol: 'openai-chat',
  },
  {
    preset: 'image',
    icon: Sparkles,
    title: 'Image API',
    description: 'OpenAI-compatible image generation request for image models.',
    protocol: 'image',
  },
  {
    preset: 'video',
    icon: Sparkles,
    title: 'Video API',
    description: 'OpenAI-compatible asynchronous video generation request.',
    protocol: 'video',
  },
  {
    preset: 'embeddings',
    icon: Layers3,
    title: 'Embeddings API',
    description: 'OpenAI-compatible text embedding request.',
    protocol: 'embeddings',
  },
  {
    preset: 'rerank',
    icon: Layers3,
    title: 'Rerank API',
    description: 'Rerank documents with a model enabled for this token.',
    protocol: 'rerank',
  },
]

function getApiBaseUrl(): string {
  if (typeof window === 'undefined') return '/v1'
  return `${window.location.origin}/v1`
}

function downloadTemplate(fileName: string, content: string) {
  if (typeof window === 'undefined') return
  const url = URL.createObjectURL(new Blob([content], { type: 'text/plain' }))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  anchor.click()
  URL.revokeObjectURL(url)
}

function ModelBadges({ model }: { model: IntegrationModel }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-1.5'>
      {model.protocols.map((protocol) => (
        <Badge key={protocol} variant='outline' className='text-[11px]'>
          {t(protocolLabel(protocol))}
        </Badge>
      ))}
    </div>
  )
}

function ClientSetup({
  preset,
  modelName,
  model,
  apiBaseUrl,
}: {
  preset: ClientPreset
  modelName: string
  model?: IntegrationModel
  apiBaseUrl: string
}) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const template = useMemo(
    () => buildClientTemplate(preset, modelName, apiBaseUrl, model),
    [apiBaseUrl, model, modelName, preset]
  )
  const copied = copiedText === template.content

  return (
    <Card className='overflow-hidden'>
      <CardHeader className='gap-2 border-b'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='flex min-w-0 items-start gap-3'>
            <span className='bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-xl'>
              <Terminal className='size-5' aria-hidden='true' />
            </span>
            <div className='min-w-0'>
              <CardTitle>{template.title}</CardTitle>
              <CardDescription className='mt-1'>
                {t(template.description)}
              </CardDescription>
            </div>
          </div>
          <div className='flex shrink-0 gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => void copyToClipboard(template.content)}
            >
              {copied ? (
                <Check data-icon='inline-start' />
              ) : (
                <Clipboard data-icon='inline-start' />
              )}
              {copied ? t('Copied') : t('Copy')}
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() =>
                downloadTemplate(template.fileName, template.content)
              }
            >
              <Download data-icon='inline-start' />
              {t('Download')}
            </Button>
          </div>
        </div>
        {(template.install || template.path) && (
          <div className='text-muted-foreground flex flex-wrap gap-x-5 gap-y-1 text-xs'>
            {template.install && (
              <span>
                <span className='font-medium'>{t('Install')}:</span>{' '}
                <code className='text-foreground'>{template.install}</code>
              </span>
            )}
            {template.path && (
              <span>
                <span className='font-medium'>{t('Config path')}:</span>{' '}
                <code className='text-foreground'>{template.path}</code>
              </span>
            )}
          </div>
        )}
      </CardHeader>
      <CardContent className='space-y-3 pt-4'>
        <div className='bg-muted/35 flex min-w-0 items-center gap-2 rounded-lg border px-3 py-2 text-xs'>
          <span className='text-muted-foreground shrink-0'>
            {t('Endpoint')}
          </span>
          <code
            className='min-w-0 flex-1 truncate font-mono'
            title={template.endpoint}
          >
            {template.endpoint}
          </code>
        </div>
        <pre className='bg-muted/35 text-foreground max-h-80 overflow-auto rounded-xl border p-3 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap'>
          <code>{template.content}</code>
        </pre>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Replace YOUR_API_KEY locally. It is never embedded in downloaded files.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}

export function Skills() {
  const { t } = useTranslation()
  const apiBaseUrl = getApiBaseUrl()
  const modelsQuery = useQuery({
    queryKey: ['integration-models'],
    queryFn: getIntegrationModels,
    staleTime: 60 * 1000,
    retry: 1,
  })
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data])
  const [search, setSearch] = useState('')
  const [modelName, setModelName] = useState('')
  const [preset, setPreset] = useState<ClientPreset>('codex')

  useEffect(() => {
    if (!modelName && models[0]) setModelName(models[0].modelName)
  }, [modelName, models])

  const selectedModel = useMemo(
    () => models.find((model) => model.modelName === modelName),
    [modelName, models]
  )
  const presets = useMemo(
    () => compatiblePresets(selectedModel),
    [selectedModel]
  )

  useEffect(() => {
    if (!presets.includes(preset)) setPreset(presets[0] ?? 'curl')
  }, [preset, presets])

  const filteredModels = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return models
    return models.filter((model) =>
      `${model.modelName} ${model.vendorName ?? ''} ${model.endpointTypes.join(' ')}`
        .toLowerCase()
        .includes(query)
    )
  }, [models, search])

  return (
    <PublicLayout>
      <div className='mx-auto w-full max-w-6xl space-y-8 py-4 sm:py-8'>
        <header className='max-w-3xl space-y-3'>
          <div className='text-primary flex items-center gap-2 text-xs font-medium tracking-[0.18em] uppercase'>
            <Sparkles className='size-3.5' aria-hidden='true' />
            Benefit API
          </div>
          <h1 className='text-3xl font-semibold tracking-tight sm:text-4xl'>
            {t('Skill Center')}
          </h1>
          <p className='text-muted-foreground text-sm leading-relaxed sm:text-base'>
            {t(
              'Copy safe, model-aware setup files for Codex, Claude Code, SDKs, and API clients.'
            )}
          </p>
          <div className='flex flex-wrap gap-2 pt-1'>
            <Button variant='outline' size='sm' render={<Link to='/docs' />}>
              {t('Open Docs')}
              <ArrowRight data-icon='inline-end' />
            </Button>
            <Button variant='ghost' size='sm' render={<Link to='/pricing' />}>
              {t('Browse models')}
              <ExternalLink data-icon='inline-end' />
            </Button>
          </div>
        </header>

        <section className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
          {presetMeta.map((item) => {
            const Icon = item.icon
            const enabled = presets.includes(item.preset)
            return (
              <Card key={item.preset} className='flex h-full flex-col'>
                <CardHeader className='pb-3'>
                  <div className='flex items-center justify-between gap-3'>
                    <span className='bg-muted flex size-9 items-center justify-center rounded-lg'>
                      <Icon className='size-4' aria-hidden='true' />
                    </span>
                    <Badge variant={enabled ? 'secondary' : 'outline'}>
                      {enabled ? t('Ready') : t('Choose a compatible model')}
                    </Badge>
                  </div>
                  <CardTitle className='text-base'>{t(item.title)}</CardTitle>
                  <CardDescription>{t(item.description)}</CardDescription>
                </CardHeader>
                <CardContent className='mt-auto pt-0'>
                  <Button
                    type='button'
                    variant={preset === item.preset ? 'default' : 'outline'}
                    size='sm'
                    className='w-full'
                    disabled={!enabled}
                    onClick={() => setPreset(item.preset)}
                  >
                    {preset === item.preset ? t('Selected') : t('Configure')}
                  </Button>
                </CardContent>
              </Card>
            )
          })}
        </section>

        <section className='grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]'>
          <div className='min-w-0 space-y-5'>
            <Card>
              <CardHeader className='gap-2'>
                <div className='flex flex-wrap items-center justify-between gap-3'>
                  <div>
                    <CardTitle>{t('Model-aware configuration')}</CardTitle>
                    <CardDescription className='mt-1'>
                      {t(
                        'Only endpoints reported by Benefit API are offered for the selected model.'
                      )}
                    </CardDescription>
                  </div>
                  {modelsQuery.isFetching && (
                    <RefreshCw className='text-muted-foreground size-4 animate-spin' />
                  )}
                </div>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]'>
                  <label className='space-y-1.5'>
                    <span className='text-sm font-medium'>{t('Client')}</span>
                    <select
                      value={preset}
                      onChange={(event) =>
                        setPreset(event.target.value as ClientPreset)
                      }
                      className='border-input bg-background h-9 w-full rounded-md border px-3 text-sm'
                    >
                      {presets.map((value) => {
                        const item = presetMeta.find(
                          (meta) => meta.preset === value
                        )
                        return (
                          <option key={value} value={value}>
                            {item ? t(item.title) : value}
                          </option>
                        )
                      })}
                    </select>
                  </label>
                  <label className='space-y-1.5'>
                    <span className='text-sm font-medium'>
                      {t('Model identifier')}
                    </span>
                    <Input
                      list='benefit-api-models'
                      value={modelName}
                      onChange={(event) => setModelName(event.target.value)}
                      placeholder='YOUR_MODEL'
                      className='font-mono text-xs'
                    />
                    <datalist id='benefit-api-models'>
                      {models.map((model) => (
                        <option key={model.modelName} value={model.modelName} />
                      ))}
                    </datalist>
                  </label>
                </div>
                {selectedModel ? (
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='text-muted-foreground text-xs'>
                      {t('Detected capabilities')}:
                    </span>
                    <ModelBadges model={selectedModel} />
                  </div>
                ) : (
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Enter a model ID manually when the public model catalog is unavailable.'
                    )}
                  </p>
                )}
              </CardContent>
            </Card>

            <ClientSetup
              preset={preset}
              modelName={modelName}
              model={selectedModel}
              apiBaseUrl={apiBaseUrl}
            />
          </div>

          <aside className='space-y-5'>
            <Card>
              <CardHeader className='gap-2'>
                <CardTitle className='text-base'>
                  {t('Available models')}
                </CardTitle>
                <CardDescription>
                  {modelsQuery.isError
                    ? t('Model catalog is temporarily unavailable.')
                    : t('{{count}} models reported by this site.', {
                        count: models.length,
                      })}
                </CardDescription>
                <div className='relative pt-1'>
                  <Search className='text-muted-foreground pointer-events-none absolute top-3 left-3 size-4' />
                  <Input
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder={t('Search models')}
                    className='pl-9'
                  />
                </div>
              </CardHeader>
              <CardContent className='max-h-[26rem] space-y-2 overflow-y-auto pt-0'>
                {filteredModels.length === 0 ? (
                  <p className='text-muted-foreground py-6 text-center text-xs'>
                    {t('No matching models.')}
                  </p>
                ) : (
                  filteredModels.map((model) => (
                    <button
                      key={model.modelName}
                      type='button'
                      className='hover:bg-muted/70 focus-visible:ring-ring w-full rounded-lg border p-3 text-left transition focus-visible:ring-2 focus-visible:outline-none'
                      onClick={() => setModelName(model.modelName)}
                    >
                      <span className='flex items-center justify-between gap-2'>
                        <span className='min-w-0 truncate font-mono text-xs'>
                          {model.modelName}
                        </span>
                        {model.modelName === modelName && (
                          <Check className='text-primary size-4 shrink-0' />
                        )}
                      </span>
                      <span className='text-muted-foreground mt-1 block truncate text-[11px]'>
                        {model.vendorName || t('Configured model')}
                      </span>
                    </button>
                  ))
                )}
              </CardContent>
            </Card>

            <Card className='bg-muted/30'>
              <CardContent className='space-y-3 p-4 text-xs leading-relaxed'>
                <p className='font-medium'>{t('Security note')}</p>
                <p className='text-muted-foreground'>
                  {t(
                    'Downloaded files contain placeholders only. Keep BENEFIT_API_KEY, ANTHROPIC_AUTH_TOKEN, and other secrets in your local environment.'
                  )}
                </p>
                <Link
                  to='/keys'
                  className='text-foreground inline-flex items-center gap-1 underline-offset-4 hover:underline'
                >
                  {t('Create an API key')}
                  <ArrowRight className='size-3.5' aria-hidden='true' />
                </Link>
              </CardContent>
            </Card>
          </aside>
        </section>
      </div>
    </PublicLayout>
  )
}
