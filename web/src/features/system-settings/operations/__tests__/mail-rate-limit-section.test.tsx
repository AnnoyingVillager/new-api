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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import {
  DEFAULT_MAIL_RATE_LIMIT,
  createMailRateLimitSchema,
  parseMailRateLimitConfig,
} from '../mail-rate-limit-schema'
import { MailRateLimitSection } from '../mail-rate-limit-section'

function Fixture({ raw }: { raw: string }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <div ref={setActions} />
      <MailRateLimitSection defaultValues={{ 'fork.mail_rate_limit': raw }} />
    </SettingsPageProvider>
  )
}

function renderSection(raw = JSON.stringify(DEFAULT_MAIL_RATE_LIMIT)) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <Fixture raw={raw} />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

afterEach(() => vi.restoreAllMocks())

describe('mail rate limit settings', () => {
  it('saves all thresholds as one namespaced option', async () => {
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const user = renderSection()

    fireEvent.change(
      screen.getByRole('spinbutton', {
        name: 'Registration verification limit',
      }),
      { target: { value: '150' } }
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'All-email hard limit' }),
      { target: { value: '280' } }
    )
    await user.click(
      screen.getByRole('button', { name: 'Save email rate limits' })
    )

    await waitFor(() =>
      expect(request).toHaveBeenCalledExactlyOnceWith('/api/option/', {
        key: 'fork.mail_rate_limit',
        value: '{"enabled":true,"registration_limit":150,"hard_limit":280}',
      })
    )
  })

  it('rejects a registration threshold that is not below the hard limit', async () => {
    const request = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const user = renderSection()
    const input = screen.getByRole('spinbutton', {
      name: 'Registration verification limit',
    })

    fireEvent.change(input, { target: { value: '290' } })
    await user.click(
      screen.getByRole('button', { name: 'Save email rate limits' })
    )

    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'true'))
    expect(request).not.toHaveBeenCalled()
  })

  it('validates the UTC-day quota boundaries at the form boundary', () => {
    const schema = createMailRateLimitSchema((key) => key)
    expect(
      schema.safeParse({
        enabled: true,
        registration_limit: 200,
        hard_limit: 290,
      }).success
    ).toBe(true)
    expect(
      schema.safeParse({
        enabled: true,
        registration_limit: 200,
        hard_limit: 301,
      }).success
    ).toBe(false)
  })

  it('falls back from malformed stored JSON without failing the page', () => {
    expect(parseMailRateLimitConfig('{')).toEqual(DEFAULT_MAIL_RATE_LIMIT)
  })
})
