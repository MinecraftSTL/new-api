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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

type QuotaAdjustOperator = '=' | '+=' | '-='

const target: User = {
  id: 2,
  username: 'managed-user',
  display_name: 'Managed user',
  role: 1,
  status: 1,
  quota: 100,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  admin_permissions: {},
}

function renderDrawer() {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/authz/catalog') {
      return {
        data: {
          success: true,
          data: { resources: [], roles: [] },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: target } }
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true },
  })

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open
          onOpenChange={() => undefined}
          currentRow={target}
        />
      </UsersProvider>
    </QueryClientProvider>
  )

  return { put }
}

async function selectOperator(operator: QuotaAdjustOperator) {
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Mode' }))
  await user.click(await screen.findByRole('option', { name: operator }))
}

beforeEach(() => {
  useAuthStore.getState().auth.reset()
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'TOKENS',
    },
  })
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

it('stages quota edits inline and keeps the amount when the operator changes', async () => {
  const { put } = renderDrawer()
  await screen.findByDisplayValue('Managed user')

  expect(
    screen.queryByRole('button', { name: 'Adjust Quota' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('dialog', { name: 'Adjust Quota' })
  ).not.toBeInTheDocument()

  const user = userEvent.setup()
  const amount = screen.getByRole('spinbutton', { name: 'Amount' })
  await user.type(amount, '-5')
  expect(screen.getByText('Current quota: 100 → -5')).toBeVisible()

  await selectOperator('+=')
  expect(amount).toHaveValue(-5)
  expect(screen.getByText('Current quota: 100 - 5 = 95')).toBeVisible()

  await selectOperator('-=')
  expect(amount).toHaveValue(-5)
  expect(screen.getByText('Current quota: 100 + 5 = 105')).toBeVisible()
  expect(put).not.toHaveBeenCalled()
})

it.each<[QuotaAdjustOperator, string, { mode: string; value: number }]>([
  ['+=', '-5', { mode: 'subtract', value: 5 }],
  ['-=', '5', { mode: 'subtract', value: 5 }],
  ['-=', '-5', { mode: 'add', value: 5 }],
  ['=', '-5', { mode: 'override', value: -5 }],
  ['=', '0', { mode: 'override', value: 0 }],
])(
  'sends %s with amount %s in the single save request',
  async (operator, amount, expectedAdjustment) => {
    const { put } = renderDrawer()
    await screen.findByDisplayValue('Managed user')

    const user = userEvent.setup()
    if (operator !== '=') {
      await selectOperator(operator)
    }
    await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), amount)
    expect(put).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith(
        '/api/user/',
        expect.objectContaining({
          id: 2,
          quota_adjustment: expectedAdjustment,
        })
      )
    )
    expect(put).toHaveBeenCalledTimes(1)
  }
)

it.each<[QuotaAdjustOperator, string]>([
  ['=', ''],
  ['+=', '0'],
])('omits quota_adjustment for %s with amount %s', async (operator, amount) => {
  const { put } = renderDrawer()
  await screen.findByDisplayValue('Managed user')

  const user = userEvent.setup()
  if (operator !== '=') {
    await selectOperator(operator)
  }
  if (amount) {
    await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), amount)
  }

  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const payload = put.mock.calls[0][1] as Record<string, unknown>
  expect(payload).not.toHaveProperty('quota_adjustment')
})

it('keeps the drawer open when the combined save fails', async () => {
  const { put } = renderDrawer()
  await screen.findByDisplayValue('Managed user')
  put.mockResolvedValueOnce({
    data: { success: false, message: 'combined save failed' },
  })

  const user = userEvent.setup()
  await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), '5')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const payload = put.mock.calls[0][1] as Record<string, unknown>
  expect(payload.quota_adjustment).toEqual({
    mode: 'override',
    value: 5,
  })
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeVisible()
  expect(
    screen.queryByText('Quota adjusted successfully')
  ).not.toBeInTheDocument()
})
