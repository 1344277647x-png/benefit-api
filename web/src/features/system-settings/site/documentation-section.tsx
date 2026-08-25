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
import { Eye, PencilLine } from 'lucide-react'
import type { Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { RichContent } from '@/components/rich-content'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const documentationSchema = z.object({
  DocsContent: z.string(),
})

type DocumentationFormValues = z.infer<typeof documentationSchema>

type DocumentationSectionProps = {
  defaultValue: string
}

export function DocumentationSection({
  defaultValue,
}: DocumentationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<DocumentationFormValues>({
      resolver: zodResolver(documentationSchema) as Resolver<
        DocumentationFormValues,
        unknown,
        DocumentationFormValues
      >,
      defaultValues: {
        DocsContent: defaultValue ?? '',
      },
      onSubmit: async (data) => {
        await updateOption.mutateAsync({
          key: 'DocsContent',
          value: data.DocsContent,
        })
      },
    })
  const content = form.watch('DocsContent')
  const isSaving = isSubmitting || updateOption.isPending

  return (
    <SettingsSection title={t('Docs')}>
      <FormNavigationGuard when={isDirty} />
      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={isSaving}
            isSaveDisabled={!isDirty}
            isResetDisabled={!isDirty}
          />

          <div data-settings-form-span='full' className='min-w-0'>
            <Tabs defaultValue='edit' className='gap-4'>
              <TabsList>
                <TabsTrigger value='edit'>
                  <PencilLine data-icon='inline-start' />
                  {t('Edit')}
                </TabsTrigger>
                <TabsTrigger value='preview'>
                  <Eye data-icon='inline-start' />
                  {t('Preview')}
                </TabsTrigger>
              </TabsList>

              <TabsContent value='edit'>
                <FormField
                  control={form.control}
                  name='DocsContent'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Documentation content')}</FormLabel>
                      <FormControl>
                        <Textarea
                          rows={22}
                          className='min-h-[28rem] resize-y font-mono text-sm leading-relaxed'
                          placeholder={t('Write your API guide in Markdown...')}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Markdown supports headings, lists, links, tables, and code blocks.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </TabsContent>

              <TabsContent value='preview'>
                <div className='bg-background min-h-[28rem] rounded-lg border p-5 md:p-7'>
                  {content.trim() ? (
                    <RichContent
                      mode='markdown'
                      content={content}
                      className='prose-neutral dark:prose-invert max-w-none'
                    />
                  ) : (
                    <div className='text-muted-foreground flex min-h-64 items-center justify-center text-sm'>
                      {t('No content to preview.')}
                    </div>
                  )}
                </div>
              </TabsContent>
            </Tabs>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
