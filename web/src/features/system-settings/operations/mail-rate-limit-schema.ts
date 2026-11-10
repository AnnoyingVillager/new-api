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
// FORK-MAIL-RATE-LIMIT
import * as z from 'zod'

export const MAIL_RATE_LIMIT_OPTION_KEY = 'fork.mail_rate_limit'

export type MailRateLimitConfig = {
  enabled: boolean
  registration_limit: number
  hard_limit: number
}

export const DEFAULT_MAIL_RATE_LIMIT: MailRateLimitConfig = {
  enabled: true,
  registration_limit: 200,
  hard_limit: 290,
}

export function parseMailRateLimitConfig(raw: string): MailRateLimitConfig {
  try {
    const value = JSON.parse(raw) as Partial<MailRateLimitConfig>
    if (
      typeof value.enabled === 'boolean' &&
      typeof value.registration_limit === 'number' &&
      typeof value.hard_limit === 'number' &&
      Number.isInteger(value.registration_limit) &&
      Number.isInteger(value.hard_limit) &&
      value.registration_limit > 0 &&
      value.registration_limit < value.hard_limit &&
      value.hard_limit <= 300
    ) {
      return {
        enabled: value.enabled,
        registration_limit: value.registration_limit,
        hard_limit: value.hard_limit,
      }
    }
  } catch {
    // A malformed saved value falls back to editable defaults. Saving repairs it.
  }
  return DEFAULT_MAIL_RATE_LIMIT
}

export function createMailRateLimitSchema(t: (key: string) => string) {
  return z
    .object({
      enabled: z.boolean(),
      registration_limit: z
        .number()
        .int(t('Enter a whole number'))
        .min(1, t('Must be greater than 0'))
        .max(299, t('Must be less than the hard limit')),
      hard_limit: z
        .number()
        .int(t('Enter a whole number'))
        .min(1, t('Must be greater than 0'))
        .max(300, t('Hard limit cannot exceed 300')),
    })
    .superRefine((values, context) => {
      if (values.registration_limit >= values.hard_limit) {
        context.addIssue({
          code: 'custom',
          path: ['registration_limit'],
          message: t('Must be less than the hard limit'),
        })
      }
    })
}

export function serializeMailRateLimitConfig(
  values: MailRateLimitConfig
): string {
  return JSON.stringify({
    enabled: values.enabled,
    registration_limit: values.registration_limit,
    hard_limit: values.hard_limit,
  })
}
