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
export const MAX_REFERENCE_IMAGE_BYTES = 20 * 1024 * 1024
export const MAX_REFERENCE_IMAGE_FILES = 4
export const MAX_REFERENCE_IMAGE_TOTAL_BYTES = 40 * 1024 * 1024

const SUPPORTED_REFERENCE_IMAGE_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/webp',
])

export type ReferenceFileSelection = {
  files: File[]
  overflowCount: number
  oversizedCount: number
  unsupportedCount: number
  totalSizeExceededCount: number
}

export function referenceImageFileKey(file: File): string {
  return `${file.name}:${file.size}:${file.type}:${file.lastModified}`
}

export function appendReferenceImageFiles(
  currentFiles: File[],
  selectedFiles: File[],
  maxFiles = MAX_REFERENCE_IMAGE_FILES,
  maxBytes = MAX_REFERENCE_IMAGE_BYTES,
  maxTotalBytes = MAX_REFERENCE_IMAGE_TOTAL_BYTES
): ReferenceFileSelection {
  const existingKeys = new Set(currentFiles.map(referenceImageFileKey))
  const validFiles: File[] = []
  let oversizedCount = 0
  let unsupportedCount = 0

  for (const file of selectedFiles) {
    if (!SUPPORTED_REFERENCE_IMAGE_TYPES.has(file.type)) {
      unsupportedCount++
      continue
    }
    if (file.size > maxBytes) {
      oversizedCount++
      continue
    }
    const key = referenceImageFileKey(file)
    if (existingKeys.has(key)) continue
    existingKeys.add(key)
    validFiles.push(file)
  }

  const availableSlots = Math.max(0, maxFiles - currentFiles.length)
  const acceptedFiles: File[] = []
  let totalSizeExceededCount = 0
  let totalBytes = currentFiles.reduce((sum, file) => sum + file.size, 0)
  for (const file of validFiles.slice(0, availableSlots)) {
    if (totalBytes + file.size > maxTotalBytes) {
      totalSizeExceededCount++
      continue
    }
    acceptedFiles.push(file)
    totalBytes += file.size
  }
  return {
    files: [...currentFiles, ...acceptedFiles],
    overflowCount: Math.max(0, validFiles.length - availableSlots),
    oversizedCount,
    unsupportedCount,
    totalSizeExceededCount,
  }
}
