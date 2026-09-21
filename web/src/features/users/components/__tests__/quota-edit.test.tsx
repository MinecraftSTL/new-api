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
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
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
  const post = vi.spyOn(api, 'post').mockResolvedValue({
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

  return { get, put, post }
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
  const { post } = renderDrawer()
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
  expect(post).not.toHaveBeenCalled()
})

it.each<[QuotaAdjustOperator, string, { mode: string; value: number }]>([
  ['+=', '-5', { mode: 'subtract', value: 5 }],
  ['-=', '5', { mode: 'subtract', value: 5 }],
  ['-=', '-5', { mode: 'add', value: 5 }],
  ['=', '-5', { mode: 'override', value: -5 }],
  ['=', '0', { mode: 'override', value: 0 }],
])(
  'applies %s with amount %s only when saving',
  async (operator, amount, expectedAdjustment) => {
    const { post } = renderDrawer()
    await screen.findByDisplayValue('Managed user')

    const user = userEvent.setup()
    if (operator !== '=') {
      await selectOperator(operator)
    }
    await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), amount)
    expect(post).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/user/manage', {
        id: 2,
        action: 'add_quota',
        ...expectedAdjustment,
      })
    )
    expect(post).toHaveBeenCalledTimes(1)
  }
)

it.each<[QuotaAdjustOperator, string]>([
  ['=', ''],
  ['+=', '0'],
])(
  'does not send a quota adjustment for %s with amount %s',
  async (operator, amount) => {
    const { put, post } = renderDrawer()
    await screen.findByDisplayValue('Managed user')

    const user = userEvent.setup()
    if (operator !== '=') {
      await selectOperator(operator)
    }
    if (amount) {
      await user.type(
        screen.getByRole('spinbutton', { name: 'Amount' }),
        amount
      )
    }

    await user.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
    expect(post).not.toHaveBeenCalled()
  }
)

it('does not adjust quota when user update fails', async () => {
  const { put, post } = renderDrawer()
  await screen.findByDisplayValue('Managed user')
  put.mockResolvedValueOnce({
    data: { success: false, message: 'user update failed' },
  })

  const user = userEvent.setup()
  await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), '5')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(post).not.toHaveBeenCalled()
})

it('keeps the drawer open when the quota adjustment fails', async () => {
  const { post } = renderDrawer()
  await screen.findByDisplayValue('Managed user')
  post.mockResolvedValueOnce({
    data: { success: false, message: 'quota adjustment failed' },
  })

  const user = userEvent.setup()
  await user.type(screen.getByRole('spinbutton', { name: 'Amount' }), '5')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))

  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeVisible()
  expect(
    screen.queryByText('Quota adjusted successfully')
  ).not.toBeInTheDocument()
})
