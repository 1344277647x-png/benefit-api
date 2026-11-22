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
import { Images, Upload, X } from 'lucide-react'
import { useEffect, useState, type ChangeEvent, type RefObject } from 'react'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'

import { referenceImageFileKey } from './reference-image-files'

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function ReferenceImageThumbnail(props: { file: File; alt: string }) {
  const [preview, setPreview] = useState('')

  useEffect(() => {
    const url = URL.createObjectURL(props.file)
    setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [props.file])

  if (!preview) {
    return <div className='bg-muted size-16 shrink-0 rounded-lg' />
  }
  return (
    <img
      src={preview}
      alt={props.alt}
      className='bg-muted size-16 shrink-0 rounded-lg border object-cover'
    />
  )
}

export function ReferenceImagePicker(props: {
  files: File[]
  maxFiles: number
  maxTotalBytes: number
  inputRef: RefObject<HTMLInputElement | null>
  onChange: (event: ChangeEvent<HTMLInputElement>) => void
  onRemove: (index: number) => void
  onClear: () => void
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  const isFull = props.files.length >= props.maxFiles

  return (
    <div className='space-y-2' data-testid='reference-image-picker'>
      <div className='flex min-h-11 flex-wrap items-center justify-between gap-2'>
        <div>
          <Label>{props.t('Reference images')}</Label>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {props.t(
              'Upload up to {{count}} reference images, no more than 20 MB each and {{total}} MB in total.',
              {
                count: props.maxFiles,
                total: Math.round(props.maxTotalBytes / 1024 / 1024),
              }
            )}
          </p>
        </div>
        {props.files.length > 0 && (
          <Button
            type='button'
            variant='ghost'
            className='min-h-11'
            onClick={props.onClear}
          >
            <X aria-hidden='true' />
            {props.t('Clear all')}
          </Button>
        )}
      </div>
      <input
        ref={props.inputRef}
        type='file'
        multiple
        accept='image/png,image/jpeg,image/webp'
        className='sr-only'
        aria-label={props.t('Choose reference images')}
        onChange={props.onChange}
      />
      {props.files.length > 0 && (
        <div className='grid min-w-0 gap-2 sm:grid-cols-2'>
          {props.files.map((file, index) => (
            <div
              key={referenceImageFileKey(file)}
              className='bg-background/45 flex min-w-0 items-center gap-2 rounded-xl border border-white/10 p-2 shadow-sm backdrop-blur-xl'
            >
              <ReferenceImageThumbnail
                file={file}
                alt={props.t('Reference image {{number}}', {
                  number: index + 1,
                })}
              />
              <div className='min-w-0 flex-1'>
                <p className='truncate text-sm font-medium'>{file.name}</p>
                <p className='text-muted-foreground text-xs'>
                  {formatBytes(file.size)}
                </p>
              </div>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='size-11 shrink-0'
                onClick={() => props.onRemove(index)}
                aria-label={props.t('Remove reference image {{number}}', {
                  number: index + 1,
                })}
              >
                <X aria-hidden='true' />
              </Button>
            </div>
          ))}
        </div>
      )}
      <Button
        type='button'
        variant='outline'
        className='min-h-11 w-full'
        disabled={isFull}
        onClick={() => props.inputRef.current?.click()}
      >
        {props.files.length > 0 ? (
          <Images aria-hidden='true' />
        ) : (
          <Upload aria-hidden='true' />
        )}
        {props.files.length > 0
          ? props.t('Add more reference images')
          : props.t('Add reference images')}
        <span className='text-muted-foreground ml-auto text-xs tabular-nums'>
          {props.files.length}/{props.maxFiles}
        </span>
      </Button>
    </div>
  )
}
