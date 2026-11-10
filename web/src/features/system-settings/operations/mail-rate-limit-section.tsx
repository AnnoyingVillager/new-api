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
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  MAIL_RATE_LIMIT_OPTION_KEY,
  createMailRateLimitSchema,
  parseMailRateLimitConfig,
  serializeMailRateLimitConfig,
  type MailRateLimitConfig,
} from './mail-rate-limit-schema'

// FORK-MAIL-RATE-LIMIT
type MailRateLimitSectionProps = {
  defaultValues: {
    'fork.mail_rate_limit': string
  }
}

export function MailRateLimitSection({
  defaultValues,
}: MailRateLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const rawConfig = defaultValues[MAIL_RATE_LIMIT_OPTION_KEY]
  const formDefaults = useMemo(
    () => parseMailRateLimitConfig(rawConfig),
    [rawConfig]
  )
  const form = useForm<MailRateLimitConfig>({
    resolver: zodResolver(createMailRateLimitSchema(t)),
    defaultValues: formDefaults,
  })
  const isSaving = form.formState.isSubmitting || updateOption.isPending
  useResetForm(form, formDefaults)

  const onSubmit = async (values: MailRateLimitConfig) => {
    const serialized = serializeMailRateLimitConfig(values)
    if (serialized === serializeMailRateLimitConfig(formDefaults)) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({
      key: MAIL_RATE_LIMIT_OPTION_KEY,
      value: serialized,
    })
  }

  return (
    <SettingsSection title={t('Email rate limits')} className='gap-6'>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSaving}
            saveLabel='Save email rate limits'
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable daily email limits')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Counts recipients across SMTP and Resend. Disabled means no fork quota is enforced.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={isSaving}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='registration_limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Registration verification limit')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={299}
                      step={1}
                      disabled={isSaving}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Registration verification email is rejected when this many recipients have been used.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='hard_limit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('All-email hard limit')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={2}
                      max={300}
                      step={1}
                      disabled={isSaving}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'At this recipient count, all email is rejected until the next UTC day.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>

          <div className='bg-muted/20 rounded-xl border px-4 py-3 text-sm'>
            <p className='font-medium'>
              {t('Resets daily at 00:00 UTC (08:00 Asia/Shanghai).')}
            </p>
            <p className='text-muted-foreground mt-1'>
              {t(
                'Limits count recipients, not send requests. A request that would cross a threshold is rejected without consuming quota. Redis is required while enabled.'
              )}
            </p>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
