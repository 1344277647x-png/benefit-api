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
import * as z from 'zod'

export const MAX_ANNOUNCEMENT_CONTENT_CHARACTERS = 10_000

export function countAnnouncementCharacters(value: string): number {
  return value.length
}

export const announcementSchema = z.object({
  content: z
    .string()
    .min(1, 'Content is required')
    .refine(
      (value) =>
        countAnnouncementCharacters(value) <=
        MAX_ANNOUNCEMENT_CONTENT_CHARACTERS,
      'Announcement content must not exceed 10,000 characters.'
    ),
  publishDate: z.string().min(1, 'Publish date is required'),
  type: z.enum(['default', 'ongoing', 'success', 'warning', 'error']),
  extra: z
    .string()
    .max(100, 'Extra must be less than 100 characters')
    .optional(),
  popupEnabled: z.boolean(),
  popupFrequency: z.enum(['once', 'daily', 'session']),
})

export type AnnouncementFormValues = z.infer<typeof announcementSchema>
