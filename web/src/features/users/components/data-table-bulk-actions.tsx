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
import { useMutation, useQuery } from '@tanstack/react-query'
import type { Row, Table } from '@tanstack/react-table'
import { Loader2, Power, PowerOff, Settings2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { batchUpdateUsers, getGroups } from '../api'
import { DEFAULT_GROUP, USER_STATUS } from '../constants'
import type { BatchUpdateUsersPayload, QuotaAdjustMode, User } from '../types'
import { useUsers } from './users-provider'

type UserBatchField = 'status' | 'balance' | 'group'

type UsersBatchEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  selectedRows: Row<User>[]
  isSubmitting: boolean
  onSubmit: (payload: BatchUpdateUsersPayload) => void
}

function UsersBatchEditDialog({
  open,
  onOpenChange,
  selectedRows,
  isSubmitting,
  onSubmit,
}: UsersBatchEditDialogProps) {
  const { t } = useTranslation()
  const [sourceId, setSourceId] = useState('manual')
  const [fields, setFields] = useState<Record<UserBatchField, boolean>>({
    status: false,
    balance: false,
    group: false,
  })
  const [status, setStatus] = useState(String(USER_STATUS.ENABLED))
  const [quotaMode, setQuotaMode] = useState<QuotaAdjustMode>('add')
  const [quotaAmount, setQuotaAmount] = useState('')
  const [group, setGroup] = useState<string>(DEFAULT_GROUP)

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    enabled: open,
    staleTime: 5 * 60 * 1000,
  })
  const groups = groupsData?.data ?? []
  const sourceRow = selectedRows.find(
    (row) => String(row.original.id) === sourceId
  )
  const source = sourceRow?.original
  const availableGroups = [
    ...new Set<string>([...groups, source?.group ?? '', group].filter(Boolean)),
  ]

  const toggleField = (field: UserBatchField, checked: boolean) => {
    setFields((previous) => ({ ...previous, [field]: checked }))
  }

  const handleSubmit = () => {
    const selectedFields = Object.values(fields).some(Boolean)
    if (!selectedFields) {
      toast.warning(t('Select at least one field to update'))
      return
    }

    const payload: BatchUpdateUsersPayload = {
      ids: selectedRows.map((row) => row.original.id),
    }
    if (fields.status) {
      payload.status = Number(resolvedStatus)
    }
    if (fields.balance) {
      const parsedAmount = Number(quotaAmount || 0)
      if (!source && !Number.isFinite(parsedAmount)) {
        toast.error(t('Enter a valid quota amount'))
        return
      }
      const value = source
        ? source.quota
        : parseQuotaFromDollars(parsedAmount)
      const mode = source ? 'override' : quotaMode
      if (mode !== 'override' && value <= 0) {
        toast.error(t('Enter a positive quota amount'))
        return
      }
      payload.quota_adjustment = { mode, value }
    }
    if (fields.group) {
      payload.group = source ? source.group : group || DEFAULT_GROUP
    }
    onSubmit(payload)
  }

  let resolvedStatus = status
  if (source) {
    resolvedStatus =
      source.status === USER_STATUS.ENABLED
        ? String(USER_STATUS.ENABLED)
        : String(USER_STATUS.DISABLED)
  }
  const resolvedGroup = source ? source.group : group || availableGroups[0]

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Batch edit users')}
      description={t(
        'Choose the fields to overwrite for {{count}} selected users.',
        { count: selectedRows.length }
      )}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? (
              <Loader2 className='mr-2 size-4 animate-spin' />
            ) : null}
            {isSubmitting ? t('Saving...') : t('Apply changes')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='space-y-2'>
          <Label>{t('Values')}</Label>
          <Select
            value={sourceId}
            onValueChange={(value) => setSourceId(value ?? 'manual')}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value='manual'>{t('Manual values')}</SelectItem>
              {selectedRows.map((row) => (
                <SelectItem
                  key={row.original.id}
                  value={String(row.original.id)}
                >
                  {row.original.username} #{row.original.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Choose Manual values or copy selected fields from one selected user.'
            )}
          </p>
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.status}
              onCheckedChange={(value) => toggleField('status', !!value)}
            />
            {t('Status')}
          </label>
          <Select
            value={String(resolvedStatus)}
            onValueChange={(value) =>
              setStatus(
                value == null ? String(USER_STATUS.ENABLED) : String(value)
              )
            }
            disabled={Boolean(source)}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value={String(USER_STATUS.ENABLED)}>
                {t('Enabled')}
              </SelectItem>
              <SelectItem value={String(USER_STATUS.DISABLED)}>
                {t('Disabled')}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.balance}
              onCheckedChange={(value) => toggleField('balance', !!value)}
            />
            {t('Available Balance')}
          </label>
          {source ? (
            <Input value={formatQuota(source.quota)} disabled readOnly />
          ) : (
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Select
                value={quotaMode}
                onValueChange={(value) =>
                  setQuotaMode(value as QuotaAdjustMode)
                }
              >
                <SelectTrigger className='sm:w-36'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectItem value='add'>{t('Increase')}</SelectItem>
                  <SelectItem value='subtract'>{t('Decrease')}</SelectItem>
                  <SelectItem value='override'>{t('Set to')}</SelectItem>
                </SelectContent>
              </Select>
              <Input
                type='number'
                min='0'
                step='any'
                value={quotaAmount}
                onChange={(event) => setQuotaAmount(event.target.value)}
                placeholder={t('Enter quota amount')}
              />
            </div>
          )}
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.group}
              onCheckedChange={(value) => toggleField('group', !!value)}
            />
            {t('User Group')}
          </label>
          <Select
            value={resolvedGroup || DEFAULT_GROUP}
            onValueChange={(value) => setGroup(value ?? DEFAULT_GROUP)}
            disabled={Boolean(source) || availableGroups.length === 0}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              {availableGroups.map((item) => (
                <SelectItem key={item} value={item}>
                  {item}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
    </Dialog>
  )
}

type DataTableBulkActionsProps = {
  table: Table<User>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useUsers()
  const [editOpen, setEditOpen] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.map((row) => row.original.id)

  const mutation = useMutation({
    mutationFn: async (payload: BatchUpdateUsersPayload) =>
      requireServerSuccess(await batchUpdateUsers(payload)),
    onSuccess: (result) => {
      const failed = result.data?.failed ?? []
      const failedIds = new Set(failed.map((item) => String(item.id)))
      table.setRowSelection((previous) => {
        const next: Record<string, boolean> = {}
        for (const id of Object.keys(previous)) {
          if (failedIds.has(id)) next[id] = true
        }
        return next
      })
      setEditOpen(false)
      triggerRefresh()

      if (failed.length > 0) {
        toast.warning(
          t('Updated {{updated}} of {{total}} users', {
            updated: result.data?.updated ?? 0,
            total: selectedRows.length,
          })
        )
        return
      }
      toast.success(
        t('Updated {{count}} users', { count: result.data?.updated ?? 0 })
      )
    },
    onError: (error) => handleServerError(error),
  })

  if (selectedRows.length === 0) return null

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() =>
                  mutation.mutate({
                    ids: selectedIds,
                    status: USER_STATUS.ENABLED,
                  })
                }
                disabled={mutation.isPending}
                aria-label={t('Enable selected users')}
              />
            }
          >
            <Power />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected users')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() =>
                  mutation.mutate({
                    ids: selectedIds,
                    status: USER_STATUS.DISABLED,
                  })
                }
                disabled={mutation.isPending}
                aria-label={t('Disable selected users')}
              />
            }
          >
            <PowerOff />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected users')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() => setEditOpen(true)}
                disabled={mutation.isPending}
                aria-label={t('Batch edit selected users')}
              />
            }
          >
            <Settings2 />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Batch edit selected users')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      {editOpen && (
        <UsersBatchEditDialog
          open
          onOpenChange={setEditOpen}
          selectedRows={selectedRows}
          isSubmitting={mutation.isPending}
          onSubmit={(payload) => mutation.mutate(payload)}
        />
      )}
    </>
  )
}
