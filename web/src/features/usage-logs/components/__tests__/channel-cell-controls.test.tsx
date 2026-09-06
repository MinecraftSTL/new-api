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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import type {
  ComponentType,
  MouseEvent as ReactMouseEvent,
  MouseEventHandler,
  ReactElement,
  ReactNode,
} from 'react'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'

vi.mock('@/lib/lobe-icon', () => ({
  getLobeIcon: () => null,
}))

vi.mock('@/components/ui/tooltip', async () => {
  const { cloneElement, isValidElement } = await import('react')

  return {
    TooltipProvider: ({ children }: { children: ReactNode }) => children,
    Tooltip: ({ children }: { children: ReactNode }) => children,
    TooltipTrigger: ({
      children,
      render,
    }: {
      children: ReactNode
      render: ReactElement
    }) =>
      isValidElement(render) ? cloneElement(render, {}, children) : children,
    TooltipContent: ({ children }: { children: ReactNode }) => (
      <div>{children}</div>
    ),
  }
})

vi.mock('@/components/ui/popover', async () => {
  const { cloneElement, createContext, isValidElement, useContext, useState } =
    await import('react')

  const PopoverContext = createContext<
    { open: boolean; setOpen: (open: boolean) => void } | undefined
  >(undefined)

  return {
    Popover: ({ children }: { children: ReactNode }) => {
      const [open, setOpen] = useState(false)
      return (
        <PopoverContext.Provider value={{ open, setOpen }}>
          <div>{children}</div>
        </PopoverContext.Provider>
      )
    },
    PopoverTrigger: ({
      children,
      render,
    }: {
      children: ReactNode
      render: ReactElement<{ onClick?: MouseEventHandler }>
    }) => {
      const popover = useContext(PopoverContext)
      if (!isValidElement(render)) return children

      return cloneElement(
        render,
        {
          onClick: (event: ReactMouseEvent) => {
            render.props.onClick?.(event)
            popover?.setOpen(true)
          },
        },
        children
      )
    },
    PopoverContent: ({ children }: { children: ReactNode }) => {
      const popover = useContext(PopoverContext)
      return popover?.open ? (
        <div data-slot='popover-content'>{children}</div>
      ) : null
    },
  }
})

const retryLog: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: 'admin',
  token_name: 'token',
  model_name: 'gpt-test',
  quota: 1,
  prompt_tokens: 1,
  completion_tokens: 0,
  use_time: 1,
  is_stream: false,
  channel: 42,
  channel_name: 'Primary',
  token_id: 1,
  group: 'default',
  ip: '',
  other: JSON.stringify({
    admin_info: {
      use_channel: ['7', '42'],
      channel_affinity: {
        rule_name: 'sticky-rule',
        using_group: 'vip',
        key_hint: 'acct-42',
        key_fp: 'fingerprint-42',
      },
    },
  }),
  request_id: 'req-1',
  upstream_request_id: '',
}

function AffinityStateProbe() {
  const { affinityDialogOpen, affinityTarget } = useUsageLogsContext()

  return (
    <output aria-label='Affinity selection'>
      {affinityDialogOpen
        ? [
            affinityTarget?.rule_name,
            affinityTarget?.using_group,
            affinityTarget?.key_hint,
            affinityTarget?.key_fp,
          ].join('|')
        : 'closed'}
    </output>
  )
}

function ChannelCellHarness() {
  const columns = useCommonLogsColumns(true, false)
  const channelCell = columns.find((column) => column.id === 'channel')?.cell

  if (typeof channelCell !== 'function') {
    throw new Error('Channel cell was not rendered')
  }

  const ChannelCell = channelCell as ComponentType<{
    row: { original: UsageLog }
  }>

  return (
    <>
      <ChannelCell row={{ original: retryLog }} />
      <AffinityStateProbe />
    </>
  )
}

function renderChannelCell() {
  return render(
    <UsageLogsProvider>
      <ChannelCellHarness />
    </UsageLogsProvider>
  )
}

describe('admin channel cell retry and affinity controls', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Retry Chain': 'Retry Chain',
      'Channel Affinity': 'Channel Affinity',
    })
  })

  test('keeps retry and affinity as separately named controls with an isolated affinity anchor', () => {
    renderChannelCell()

    const retryButton = screen.getByRole('button', { name: 'Retry Chain' })
    const affinityButton = screen.getByRole('button', {
      name: 'Channel Affinity',
    })
    const channelBadge = screen
      .getByText('#42')
      .closest<HTMLElement>('[data-slot="status-badge"]')
    const affinityAnchor = affinityButton.parentElement

    expect(retryButton).toBeVisible()
    expect(affinityButton).toBeVisible()
    expect(affinityAnchor).toHaveClass('relative')
    expect(affinityAnchor).toContainElement(channelBadge)
    expect(affinityAnchor).not.toContainElement(retryButton)
    expect(affinityAnchor?.parentElement).toContainElement(retryButton)
  })

  test('opens the retry chain without selecting the affinity rule', async () => {
    const user = userEvent.setup()
    renderChannelCell()

    await user.click(screen.getByRole('button', { name: 'Retry Chain' }))

    const popover = await waitFor(() => {
      const element = document.querySelector('[data-slot="popover-content"]')
      expect(element).toBeInTheDocument()
      return element as HTMLElement
    })
    expect(
      within(popover).getByText(['7', '42'].join(' \u2192 '))
    ).toBeVisible()
    expect(screen.getByLabelText('Affinity selection')).toHaveTextContent(
      'closed'
    )
  })

  test('selects the affinity rule without opening the retry chain', async () => {
    const user = userEvent.setup()
    renderChannelCell()

    await user.click(screen.getByRole('button', { name: 'Channel Affinity' }))

    expect(screen.getByLabelText('Affinity selection')).toHaveTextContent(
      'sticky-rule|vip|acct-42|fingerprint-42'
    )
    expect(
      document.querySelector('[data-slot="popover-content"]')
    ).not.toBeInTheDocument()
  })
})
