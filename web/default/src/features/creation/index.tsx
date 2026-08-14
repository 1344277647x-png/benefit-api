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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  Check,
  Download,
  ImagePlus,
  Images,
  Loader2,
  MonitorPlay,
  RefreshCw,
  Sparkles,
  Trash2,
  Upload,
  Video,
  WandSparkles,
  X,
} from 'lucide-react'
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatTimestamp } from '@/lib/format'

import {
  createImage,
  createVideo,
  deleteCreationJob,
  getCreationJob,
  getCreationJobs,
  getCreationModels,
  retryCreationArchive,
  uploadCreationAsset,
} from './api'
import type {
  CreationCapabilities,
  CreationKind,
  CreationModel,
  CreationModelsData,
  CreationProtocol,
  GenerationAsset,
  GenerationJob,
  GenerationJobStatus,
} from './types'

const RUNNING_STATUSES = new Set<GenerationJobStatus>([
  'pending',
  'queued',
  'processing',
  'archiving',
])

const statusClasses: Record<GenerationJobStatus, string> = {
  pending: 'border-amber-500/30 bg-amber-500/10 text-amber-600',
  queued: 'border-sky-500/30 bg-sky-500/10 text-sky-600',
  processing: 'border-sky-500/30 bg-sky-500/10 text-sky-600',
  archiving: 'border-violet-500/30 bg-violet-500/10 text-violet-600',
  archive_failed: 'border-amber-500/30 bg-amber-500/10 text-amber-600',
  succeeded: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600',
  failed: 'border-rose-500/30 bg-rose-500/10 text-rose-600',
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function statusLabel(status: GenerationJobStatus, t: (key: string) => string) {
  const labels: Record<GenerationJobStatus, string> = {
    pending: t('Preparing'),
    queued: t('Queued'),
    processing: t('Generating'),
    archiving: t('Saving result'),
    archive_failed: t('Save failed'),
    succeeded: t('Completed'),
    failed: t('Failed'),
  }
  return labels[status]
}

function capabilityLabel(model: CreationModel, t: (key: string) => string) {
  if (model.protocol === 'gemini-image') return t('Gemini native image')
  if (model.protocol === 'imagen') return t('Imagen image')
  if (model.protocol === 'openai-video') return t('OpenAI video protocol')
  return t('OpenAI image protocol')
}

function NativeSelect({
  value,
  onChange,
  children,
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  children: React.ReactNode
  disabled?: boolean
}) {
  return (
    <select
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
      className='border-input bg-background/70 text-foreground focus-visible:border-ring focus-visible:ring-ring/50 h-8 w-full min-w-0 rounded-lg border px-2.5 text-sm transition outline-none focus-visible:ring-3 disabled:cursor-not-allowed disabled:opacity-50'
    >
      {children}
    </select>
  )
}

function JobStatusBadge({
  status,
  t,
}: {
  status: GenerationJobStatus
  t: (key: string) => string
}) {
  return (
    <Badge variant='outline' className={statusClasses[status]}>
      {RUNNING_STATUSES.has(status) && (
        <Loader2 className='size-3 animate-spin' />
      )}
      {!RUNNING_STATUSES.has(status) && status === 'succeeded' && (
        <Check className='size-3' />
      )}
      {!RUNNING_STATUSES.has(status) && status === 'failed' && (
        <X className='size-3' />
      )}
      {statusLabel(status, t)}
    </Badge>
  )
}

function AssetPreview({
  asset,
  kind,
}: {
  asset: GenerationAsset
  kind: CreationKind
}) {
  const contentUrl = asset.content_url ?? ''
  if (!contentUrl) return null
  if (kind === 'video' || asset.mime_type.startsWith('video/')) {
    return (
      <video
        controls
        preload='metadata'
        className='aspect-video w-full rounded-lg border bg-black object-contain'
        src={contentUrl}
      />
    )
  }
  return (
    <img
      src={contentUrl}
      alt=''
      className='bg-muted/30 aspect-square w-full rounded-lg border object-contain'
      loading='lazy'
    />
  )
}

function EmptyCreation({
  message,
  t,
}: {
  message: string
  t: (key: string) => string
}) {
  return (
    <div className='mx-auto flex min-h-[50vh] max-w-xl items-center justify-center px-4 py-12'>
      <Card className='bg-card/60 w-full border-dashed text-center shadow-none'>
        <CardContent className='flex flex-col items-center gap-3 py-10'>
          <div className='bg-primary/10 text-primary flex size-12 items-center justify-center rounded-2xl'>
            <WandSparkles className='size-6' />
          </div>
          <h1 className='text-lg font-semibold'>{t('AI Creation')}</h1>
          <p className='text-muted-foreground max-w-md text-sm'>{message}</p>
        </CardContent>
      </Card>
    </div>
  )
}

function StorageMeter({
  data,
  t,
}: {
  data: CreationModelsData
  t: (key: string) => string
}) {
  const usage = data.storage.user_bytes + data.storage.reserved_bytes
  const percent =
    data.storage.user_limit > 0
      ? Math.min(100, (usage / data.storage.user_limit) * 100)
      : 0
  return (
    <div className='space-y-1.5'>
      <div className='text-muted-foreground flex items-center justify-between text-xs'>
        <span>{t('Media storage')}</span>
        <span className='tabular-nums'>
          {formatBytes(usage)} / {formatBytes(data.storage.user_limit)}
        </span>
      </div>
      <Progress value={percent} className='h-1.5' />
    </div>
  )
}

export function Creation() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const modelsQuery = useQuery({
    queryKey: ['creation-models'],
    queryFn: getCreationModels,
    staleTime: 60_000,
    retry: false,
  })
  const jobsQuery = useQuery({
    queryKey: ['creation-jobs'],
    queryFn: () => getCreationJobs(1, 24),
    staleTime: 5_000,
    refetchInterval: 10_000,
    retry: false,
  })

  const [kind, setKind] = useState<CreationKind>('image')
  const [modelId, setModelId] = useState('')
  const [prompt, setPrompt] = useState('')
  const [count, setCount] = useState('1')
  const [size, setSize] = useState('')
  const [aspectRatio, setAspectRatio] = useState('')
  const [quality, setQuality] = useState('')
  const [duration, setDuration] = useState('5')
  const [resolution, setResolution] = useState('720p')
  const [referenceFile, setReferenceFile] = useState<File | null>(null)
  const [referencePreview, setReferencePreview] = useState('')
  const [activeJobId, setActiveJobId] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const data = modelsQuery.data?.data
  const models = useMemo(() => data?.models ?? [], [data?.models])
  const availableModels = useMemo(
    () => models.filter((model) => model.kind === kind),
    [kind, models]
  )
  const selectedModel =
    availableModels.find((model) => model.id === modelId) ?? availableModels[0]
  const capabilities = selectedModel?.capabilities
  const activeJobQuery = useQuery({
    queryKey: ['creation-job', activeJobId],
    queryFn: () => getCreationJob(activeJobId as string),
    enabled: Boolean(activeJobId),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status
      return status && !RUNNING_STATUSES.has(status) ? false : 2500
    },
    retry: false,
  })
  const activeJob = activeJobQuery.data?.data

  useEffect(() => {
    if (availableModels.length === 0) {
      setModelId('')
      return
    }
    if (!availableModels.some((model) => model.id === modelId)) {
      setModelId(availableModels[0].id)
    }
  }, [availableModels, modelId])

  useEffect(() => {
    if (!selectedModel) return
    const nextCapabilities = selectedModel.capabilities
    setSize(nextCapabilities.sizes?.[0] ?? '')
    setAspectRatio(nextCapabilities.aspect_ratios?.[0] ?? '')
    setQuality(nextCapabilities.qualities?.[0] ?? '')
    setCount((current) => {
      const max = nextCapabilities.max_count ?? 1
      return String(Math.min(Math.max(Number(current) || 1, 1), max))
    })
    setDuration(String(nextCapabilities.durations?.[0] ?? 5))
    setResolution(nextCapabilities.resolutions?.[0] ?? '720p')
  }, [selectedModel])

  useEffect(() => {
    if (capabilities?.reference_image || !referenceFile) return
    setReferenceFile(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }, [capabilities?.reference_image, referenceFile])

  useEffect(() => {
    if (!referenceFile) {
      setReferencePreview('')
      return
    }
    const url = URL.createObjectURL(referenceFile)
    setReferencePreview(url)
    return () => URL.revokeObjectURL(url)
  }, [referenceFile])

  const setReference = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0] ?? null
    if (!file) return
    if (!file.type.startsWith('image/')) {
      toast.error(t('Please select an image file.'))
      event.target.value = ''
      return
    }
    setReferenceFile(file)
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!selectedModel || !prompt.trim()) {
      toast.error(t('Choose a model and enter a prompt.'))
      return
    }
    setIsSubmitting(true)
    try {
      let referenceAssetId: string | undefined
      if (referenceFile && capabilities?.reference_image) {
        const uploaded = await uploadCreationAsset(referenceFile)
        if (!uploaded.success || !uploaded.data) {
          throw new Error(
            uploaded.message || t('Reference image upload failed.')
          )
        }
        referenceAssetId = uploaded.data.id
      }

      const created =
        kind === 'image'
          ? await createImage({
              model: selectedModel.id,
              protocol: selectedModel.protocol as Exclude<
                CreationProtocol,
                'openai-video'
              >,
              prompt: prompt.trim(),
              count: Number(count) || 1,
              size: size || undefined,
              aspect_ratio: aspectRatio || undefined,
              quality: quality || undefined,
              reference_asset_id: referenceAssetId,
            })
          : await createVideo({
              model: selectedModel.id,
              prompt: prompt.trim(),
              duration: Number(duration) || 5,
              resolution,
              reference_asset_id: referenceAssetId,
            })
      if (!created.success || !created.data) {
        throw new Error(created.message || t('Generation request failed.'))
      }
      setActiveJobId(created.data.id)
      setPrompt('')
      setReferenceFile(null)
      if (fileInputRef.current) fileInputRef.current.value = ''
      await queryClient.invalidateQueries({ queryKey: ['creation-jobs'] })
      toast.success(t('Generation request submitted.'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Generation request failed.')
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleDelete = async (job: GenerationJob) => {
    try {
      const result = await deleteCreationJob(job.id)
      if (!result.success) {
        throw new Error(result.message || t('Delete failed.'))
      }
      if (activeJobId === job.id) setActiveJobId(null)
      await queryClient.invalidateQueries({ queryKey: ['creation-jobs'] })
      toast.success(t('Deleted'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Delete failed.'))
    }
  }

  const handleRetryArchive = async (job: GenerationJob) => {
    try {
      const result = await retryCreationArchive(job.id)
      if (!result.success || !result.data) {
        throw new Error(result.message || t('Retry failed.'))
      }
      setActiveJobId(job.id)
      await queryClient.invalidateQueries({ queryKey: ['creation-jobs'] })
      await queryClient.invalidateQueries({
        queryKey: ['creation-job', job.id],
      })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Retry failed.'))
    }
  }

  if (modelsQuery.isLoading) {
    return (
      <Main className='overflow-auto'>
        <div className='mx-auto grid w-full max-w-6xl gap-4 p-3 sm:p-5 lg:grid-cols-[minmax(320px,0.85fr)_minmax(0,1.15fr)]'>
          <Skeleton className='h-[560px] rounded-xl' />
          <Skeleton className='h-[560px] rounded-xl' />
        </div>
      </Main>
    )
  }

  if (modelsQuery.isError || modelsQuery.data?.success === false) {
    return (
      <Main className='overflow-auto'>
        <EmptyCreation
          t={t}
          message={
            modelsQuery.data?.message ||
            t('The AI creation center is not enabled for this account yet.')
          }
        />
      </Main>
    )
  }

  const jobs = jobsQuery.data?.data?.items ?? []
  const hasReference = Boolean(referenceFile && capabilities?.reference_image)

  return (
    <Main className='overflow-auto'>
      <div className='mx-auto w-full max-w-6xl space-y-4 p-3 sm:space-y-5 sm:p-5'>
        <header className='flex flex-wrap items-end justify-between gap-3'>
          <div className='min-w-0'>
            <div className='text-primary mb-1 flex items-center gap-2 text-xs font-medium tracking-[0.18em] uppercase'>
              <Sparkles className='size-3.5' />
              Benefit API
            </div>
            <h1 className='text-2xl font-semibold tracking-tight sm:text-3xl'>
              {t('AI Creation')}
            </h1>
            <p className='text-muted-foreground mt-1 max-w-2xl text-sm'>
              {t(
                'Create images and videos with the models enabled for your account.'
              )}
            </p>
          </div>
          {data && (
            <div className='w-full max-w-xs'>
              <StorageMeter data={data} t={t} />
            </div>
          )}
        </header>

        <div className='grid min-w-0 gap-4 lg:grid-cols-[minmax(320px,0.85fr)_minmax(0,1.15fr)]'>
          <Card className='bg-card/80 min-w-0 shadow-sm'>
            <CardHeader className='border-b'>
              <CardTitle className='flex items-center gap-2'>
                <WandSparkles className='text-primary size-4' />
                {t('Create')}
              </CardTitle>
              <CardDescription>
                {t(
                  'Your request is billed using the selected model and your normal account permissions.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='pt-4'>
              <Tabs
                value={kind}
                onValueChange={(value) => setKind(value as CreationKind)}
              >
                <TabsList className='w-full'>
                  <TabsTrigger value='image'>
                    <Images />
                    {t('Image')}
                  </TabsTrigger>
                  <TabsTrigger value='video'>
                    <Video />
                    {t('Video')}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value='image' className='mt-4'>
                  <form className='space-y-4' onSubmit={handleSubmit}>
                    <CreationFormFields
                      kind='image'
                      models={availableModels}
                      modelId={selectedModel?.id ?? ''}
                      setModelId={setModelId}
                      capabilities={capabilities}
                      prompt={prompt}
                      setPrompt={setPrompt}
                      count={count}
                      setCount={setCount}
                      size={size}
                      setSize={setSize}
                      aspectRatio={aspectRatio}
                      setAspectRatio={setAspectRatio}
                      quality={quality}
                      setQuality={setQuality}
                      duration={duration}
                      setDuration={setDuration}
                      resolution={resolution}
                      setResolution={setResolution}
                      t={t}
                    />
                    <ReferencePicker
                      enabled={Boolean(capabilities?.reference_image)}
                      file={referenceFile}
                      preview={referencePreview}
                      inputRef={fileInputRef}
                      onChange={setReference}
                      onClear={() => {
                        setReferenceFile(null)
                        if (fileInputRef.current) {
                          fileInputRef.current.value = ''
                        }
                      }}
                      t={t}
                    />
                    <Button
                      type='submit'
                      className='w-full'
                      disabled={
                        isSubmitting || !selectedModel || !prompt.trim()
                      }
                    >
                      {isSubmitting ? (
                        <Loader2 className='animate-spin' />
                      ) : (
                        <Sparkles />
                      )}
                      {isSubmitting ? t('Submitting...') : t('Generate image')}
                    </Button>
                  </form>
                </TabsContent>
                <TabsContent value='video' className='mt-4'>
                  <form className='space-y-4' onSubmit={handleSubmit}>
                    <CreationFormFields
                      kind='video'
                      models={availableModels}
                      modelId={selectedModel?.id ?? ''}
                      setModelId={setModelId}
                      capabilities={capabilities}
                      prompt={prompt}
                      setPrompt={setPrompt}
                      count={count}
                      setCount={setCount}
                      size={size}
                      setSize={setSize}
                      aspectRatio={aspectRatio}
                      setAspectRatio={setAspectRatio}
                      quality={quality}
                      setQuality={setQuality}
                      duration={duration}
                      setDuration={setDuration}
                      resolution={resolution}
                      setResolution={setResolution}
                      t={t}
                    />
                    <ReferencePicker
                      enabled={Boolean(capabilities?.reference_image)}
                      file={referenceFile}
                      preview={referencePreview}
                      inputRef={fileInputRef}
                      onChange={setReference}
                      onClear={() => {
                        setReferenceFile(null)
                        if (fileInputRef.current) {
                          fileInputRef.current.value = ''
                        }
                      }}
                      t={t}
                    />
                    <Alert className='bg-muted/40'>
                      <MonitorPlay />
                      <AlertDescription>
                        {t(
                          'Video generation is asynchronous. You can leave this page and return to the history list.'
                        )}
                      </AlertDescription>
                    </Alert>
                    <Button
                      type='submit'
                      className='w-full'
                      disabled={
                        isSubmitting || !selectedModel || !prompt.trim()
                      }
                    >
                      {isSubmitting ? (
                        <Loader2 className='animate-spin' />
                      ) : (
                        <Video />
                      )}
                      {isSubmitting ? t('Submitting...') : t('Generate video')}
                    </Button>
                  </form>
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

          <Card className='bg-card/80 min-w-0 shadow-sm'>
            <CardHeader className='border-b'>
              <div className='flex items-start justify-between gap-3'>
                <div>
                  <CardTitle className='flex items-center gap-2'>
                    <ImagePlus className='text-primary size-4' />
                    {t('Result')}
                  </CardTitle>
                  <CardDescription>
                    {t(
                      'Generated files stay private and expire after {{days}} days.',
                      { days: data?.retention_days ?? 7 }
                    )}
                  </CardDescription>
                </div>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  onClick={() =>
                    void queryClient.invalidateQueries({
                      queryKey: ['creation-jobs'],
                    })
                  }
                  aria-label={t('Refresh')}
                >
                  <RefreshCw
                    className={jobsQuery.isFetching ? 'animate-spin' : ''}
                  />
                </Button>
              </div>
            </CardHeader>
            <CardContent className='space-y-4 pt-4'>
              {activeJob && (
                <ActiveJobCard
                  job={activeJob}
                  t={t}
                  onRetry={() => void handleRetryArchive(activeJob)}
                />
              )}
              {activeJobQuery.isFetching && !activeJob && (
                <Skeleton className='h-24 rounded-lg' />
              )}
              {jobs.length === 0 ? (
                <div className='text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 rounded-lg border border-dashed p-6 text-center text-sm'>
                  <Images className='size-8 opacity-50' />
                  <span>{t('Your generated results will appear here.')}</span>
                </div>
              ) : (
                <div className='space-y-3'>
                  {jobs.map((job) => (
                    <JobCard
                      key={job.id}
                      job={job}
                      t={t}
                      onSelect={() => setActiveJobId(job.id)}
                      onDelete={() => void handleDelete(job)}
                      onRetry={() => void handleRetryArchive(job)}
                    />
                  ))}
                </div>
              )}
              {hasReference && (
                <span className='sr-only'>{t('Reference image selected')}</span>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </Main>
  )
}

function CreationFormFields({
  kind,
  models,
  modelId,
  setModelId,
  capabilities,
  prompt,
  setPrompt,
  count,
  setCount,
  size,
  setSize,
  aspectRatio,
  setAspectRatio,
  quality,
  setQuality,
  duration,
  setDuration,
  resolution,
  setResolution,
  t,
}: {
  kind: CreationKind
  models: CreationModel[]
  modelId: string
  setModelId: (value: string) => void
  capabilities?: CreationCapabilities
  prompt: string
  setPrompt: (value: string) => void
  count: string
  setCount: (value: string) => void
  size: string
  setSize: (value: string) => void
  aspectRatio: string
  setAspectRatio: (value: string) => void
  quality: string
  setQuality: (value: string) => void
  duration: string
  setDuration: (value: string) => void
  resolution: string
  setResolution: (value: string) => void
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <>
      <div className='space-y-1.5'>
        <Label htmlFor={`${kind}-model`}>{t('Model')}</Label>
        <NativeSelect
          value={modelId}
          onChange={setModelId}
          disabled={models.length === 0}
        >
          {models.length === 0 ? (
            <option value=''>{t('No compatible models')}</option>
          ) : (
            models.map((model) => (
              <option key={`${model.protocol}:${model.id}`} value={model.id}>
                {model.display_name} · {capabilityLabel(model, t)}
              </option>
            ))
          )}
        </NativeSelect>
      </div>
      <div className='space-y-1.5'>
        <Label htmlFor={`${kind}-prompt`}>{t('Prompt')}</Label>
        <Textarea
          id={`${kind}-prompt`}
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          placeholder={t('Describe what you want to create...')}
          className='min-h-32 resize-y'
          maxLength={20000}
        />
        <div className='text-muted-foreground text-right text-[11px] tabular-nums'>
          {prompt.length.toLocaleString()} / 20,000
        </div>
      </div>
      {kind === 'image' ? (
        <div className='grid gap-3 sm:grid-cols-2'>
          {capabilities?.sizes && capabilities.sizes.length > 0 && (
            <FieldSelect
              label={t('Size')}
              value={size}
              onChange={setSize}
              options={capabilities.sizes}
            />
          )}
          {capabilities?.aspect_ratios &&
            capabilities.aspect_ratios.length > 0 && (
              <FieldSelect
                label={t('Aspect ratio')}
                value={aspectRatio}
                onChange={setAspectRatio}
                options={capabilities.aspect_ratios}
              />
            )}
          {capabilities?.qualities && capabilities.qualities.length > 0 && (
            <FieldSelect
              label={t('Quality')}
              value={quality}
              onChange={setQuality}
              options={capabilities.qualities}
            />
          )}
          <div className='space-y-1.5'>
            <Label htmlFor='image-count'>{t('Images')}</Label>
            <Input
              id='image-count'
              type='number'
              min={1}
              max={capabilities?.max_count ?? 4}
              value={count}
              onChange={(event) => setCount(event.target.value)}
            />
          </div>
        </div>
      ) : (
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='space-y-1.5'>
            <Label htmlFor='video-duration'>{t('Duration (seconds)')}</Label>
            <NativeSelect value={duration} onChange={setDuration}>
              {(capabilities?.durations ?? [4, 5, 6, 8, 10]).map((value) => (
                <option key={value} value={value}>
                  {value}s
                </option>
              ))}
            </NativeSelect>
          </div>
          <FieldSelect
            label={t('Resolution')}
            value={resolution}
            onChange={setResolution}
            options={capabilities?.resolutions ?? ['720p', '1080p']}
          />
        </div>
      )}
    </>
  )
}

function FieldSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: string[]
}) {
  return (
    <div className='space-y-1.5'>
      <Label>{label}</Label>
      <NativeSelect value={value} onChange={onChange}>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </NativeSelect>
    </div>
  )
}

function ReferencePicker({
  enabled,
  file,
  preview,
  inputRef,
  onChange,
  onClear,
  t,
}: {
  enabled: boolean
  file: File | null
  preview: string
  inputRef: React.RefObject<HTMLInputElement | null>
  onChange: (event: ChangeEvent<HTMLInputElement>) => void
  onClear: () => void
  t: (key: string) => string
}) {
  if (!enabled) return null
  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <Label>{t('Reference image')}</Label>
        {file && (
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            onClick={onClear}
            aria-label={t('Remove')}
          >
            <X />
          </Button>
        )}
      </div>
      <input
        ref={inputRef}
        type='file'
        accept='image/png,image/jpeg,image/webp'
        className='sr-only'
        onChange={onChange}
      />
      {file && preview ? (
        <div className='bg-muted/30 flex items-center gap-3 rounded-lg border p-2'>
          <img
            src={preview}
            alt=''
            className='size-14 rounded-md object-cover'
          />
          <div className='min-w-0 flex-1'>
            <p className='truncate text-sm font-medium'>{file.name}</p>
            <p className='text-muted-foreground text-xs'>
              {formatBytes(file.size)}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => inputRef.current?.click()}
          >
            <Upload />
            {t('Replace')}
          </Button>
        </div>
      ) : (
        <Button
          type='button'
          variant='outline'
          className='w-full'
          onClick={() => inputRef.current?.click()}
        >
          <Upload />
          {t('Add reference image')}
        </Button>
      )}
    </div>
  )
}

function ActiveJobCard({
  job,
  t,
  onRetry,
}: {
  job: GenerationJob
  t: (key: string, options?: Record<string, unknown>) => string
  onRetry: () => void
}) {
  const isRunning = RUNNING_STATUSES.has(job.status)
  return (
    <div className='bg-muted/20 space-y-3 rounded-xl border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='min-w-0'>
          <p className='truncate text-sm font-medium'>{job.model}</p>
          <p className='text-muted-foreground truncate text-xs'>{job.prompt}</p>
        </div>
        <JobStatusBadge status={job.status} t={t} />
      </div>
      {job.assets?.map((asset) => (
        <AssetPreview key={asset.id} asset={asset} kind={job.kind} />
      ))}
      {isRunning && (
        <div className='text-muted-foreground flex items-center gap-2 text-xs'>
          <Loader2 className='size-3.5 animate-spin' />
          {t('Updating task status...')}
        </div>
      )}
      {job.status === 'failed' && job.error_message && (
        <Alert variant='destructive'>
          <AlertCircle />
          <AlertDescription>{job.error_message}</AlertDescription>
        </Alert>
      )}
      {job.status === 'archive_failed' && (
        <Button size='sm' variant='outline' onClick={onRetry}>
          <RefreshCw />
          {t('Retry save')}
        </Button>
      )}
    </div>
  )
}

function JobCard({
  job,
  t,
  onSelect,
  onDelete,
  onRetry,
}: {
  job: GenerationJob
  t: (key: string, options?: Record<string, unknown>) => string
  onSelect: () => void
  onDelete: () => void
  onRetry: () => void
}) {
  const outputAssets =
    job.assets?.filter((asset) => asset.role === 'output') ?? []
  return (
    <div className='group bg-background/40 hover:bg-muted/30 flex min-w-0 gap-3 rounded-xl border p-2.5 transition'>
      <button
        type='button'
        className='min-w-0 flex-1 text-left'
        onClick={onSelect}
      >
        <div className='flex items-center gap-2'>
          <span className='text-sm font-medium'>
            {job.kind === 'video' ? t('Video') : t('Image')}
          </span>
          <JobStatusBadge status={job.status} t={t} />
        </div>
        <p className='text-muted-foreground mt-1 truncate text-xs'>
          {job.prompt}
        </p>
        <p className='text-muted-foreground mt-1 text-[11px]'>
          {formatTimestamp(job.created_at)} · {job.model}
        </p>
      </button>
      <div className='flex shrink-0 items-center gap-1'>
        {outputAssets[0]?.content_url && (
          <Button
            render={
              <a href={`${outputAssets[0].content_url}?download=1`} download />
            }
            variant='ghost'
            size='icon-sm'
            aria-label={t('Download')}
          >
            <Download />
          </Button>
        )}
        {job.status === 'archive_failed' && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onRetry}
            aria-label={t('Retry save')}
          >
            <RefreshCw />
          </Button>
        )}
        {!RUNNING_STATUSES.has(job.status) && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onDelete}
            aria-label={t('Delete')}
          >
            <Trash2 />
          </Button>
        )}
      </div>
    </div>
  )
}
