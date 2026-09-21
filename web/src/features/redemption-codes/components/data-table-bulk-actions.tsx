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
import { useMutation } from '@tanstack/react-query'
import type { Row, Table } from '@tanstack/react-table'
import { Loader2, Power, PowerOff, Settings2, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
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
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  formatQuota,
  formatTimestampToDate,
  parseQuotaFromDollars,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { batchDeleteRedemptions, batchUpdateRedemptions } from '../api'
import { REDEMPTION_STATUS } from '../constants'
import type { BatchUpdateRedemptionsPayload, Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type RedemptionBatchField = 'status' | 'name' | 'quota' | 'expiry'

type RedemptionsBatchEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  selectedRows: Row<Redemption>[]
  isSubmitting: boolean
  onSubmit: (payload: BatchUpdateRedemptionsPayload) => void
}

function RedemptionsBatchEditDialog({
  open,
  onOpenChange,
  selectedRows,
  isSubmitting,
  onSubmit,
}: RedemptionsBatchEditDialogProps) {
  const { t } = useTranslation()
  const [sourceId, setSourceId] = useState('manual')
  const [fields, setFields] = useState<Record<RedemptionBatchField, boolean>>({
    status: false,
    name: false,
    quota: false,
    expiry: false,
  })
  const [status, setStatus] = useState(String(REDEMPTION_STATUS.ENABLED))
  const [name, setName] = useState('')
  const [quotaAmount, setQuotaAmount] = useState('')
  const [expiresNever, setExpiresNever] = useState(true)
  const [expiredAt, setExpiredAt] = useState<Date | undefined>()

  const sourceRow = selectedRows.find(
    (row) => String(row.original.id) === sourceId
  )
  const source = sourceRow?.original

  const toggleField = (field: RedemptionBatchField, checked: boolean) => {
    setFields((previous) => ({ ...previous, [field]: checked }))
  }

  const handleSubmit = () => {
    const selectedFields = Object.values(fields).some(Boolean)
    if (!selectedFields) {
      toast.warning(t('Select at least one field to update'))
      return
    }
    const payload: BatchUpdateRedemptionsPayload = {
      ids: selectedRows.map((row) => row.original.id),
    }
    if (fields.status) {
      payload.status = Number(resolvedStatus)
    }
    if (fields.name) {
      const nextName = source ? source.name : name
      const nameLength = [...nextName].length
      if (nameLength === 0 || nameLength > 20) {
        toast.error(t('Redemption name must be between 1 and 20 characters'))
        return
      }
      payload.name = nextName
    }
    if (fields.quota) {
      const parsedAmount = Number(quotaAmount || 0)
      if (!source && !Number.isFinite(parsedAmount)) {
        toast.error(t('Quota must be a positive number'))
        return
      }
      const nextQuota = source
        ? source.quota
        : parseQuotaFromDollars(parsedAmount)
      if (nextQuota <= 0) {
        toast.error(t('Quota must be a positive number'))
        return
      }
      payload.quota = nextQuota
    }
    if (fields.expiry) {
      if (source) {
        payload.expired_time = source.expired_time
      } else if (expiresNever) {
        payload.expired_time = 0
      } else if (expiredAt) {
        payload.expired_time = Math.floor(expiredAt.getTime() / 1000)
      } else {
        toast.error(t('Select an expiration time'))
        return
      }
    }
    onSubmit(payload)
  }

  let resolvedStatus = status
  if (source) {
    resolvedStatus =
      source.status === REDEMPTION_STATUS.ENABLED
        ? String(REDEMPTION_STATUS.ENABLED)
        : String(REDEMPTION_STATUS.DISABLED)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Batch edit redemption codes')}
      description={t(
        'Choose the fields to overwrite for {{count}} selected redemption codes.',
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
                  {row.original.name} #{row.original.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Choose Manual values or copy selected fields from one selected redemption code.'
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
                value == null
                  ? String(REDEMPTION_STATUS.ENABLED)
                  : String(value)
              )
            }
            disabled={Boolean(source)}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value={String(REDEMPTION_STATUS.ENABLED)}>
                {t('Enabled')}
              </SelectItem>
              <SelectItem value={String(REDEMPTION_STATUS.DISABLED)}>
                {t('Disabled')}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.name}
              onCheckedChange={(value) => toggleField('name', !!value)}
            />
            {t('Name')}
          </label>
          <Input
            value={source ? source.name : name}
            onChange={(event) => setName(event.target.value)}
            disabled={Boolean(source)}
            maxLength={20}
            placeholder={t('Enter name')}
          />
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.quota}
              onCheckedChange={(value) => toggleField('quota', !!value)}
            />
            {t('Quota')}
          </label>
          {source ? (
            <Input value={formatQuota(source.quota)} disabled readOnly />
          ) : (
            <Input
              type='number'
              min='0'
              step='any'
              value={quotaAmount}
              onChange={(event) => setQuotaAmount(event.target.value)}
              placeholder={t('Enter quota amount')}
            />
          )}
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.expiry}
              onCheckedChange={(value) => toggleField('expiry', !!value)}
            />
            {t('Expiration Time')}
          </label>
          {source ? (
            <Input
              value={
                source.expired_time === 0
                  ? t('Never')
                  : formatTimestampToDate(source.expired_time)
              }
              disabled
              readOnly
            />
          ) : (
            <div className='space-y-3'>
              <label className='flex items-center justify-between gap-3 text-sm'>
                <span>{t('Never expires')}</span>
                <Switch
                  checked={expiresNever}
                  onCheckedChange={setExpiresNever}
                />
              </label>
              {!expiresNever && (
                <DateTimePicker
                  value={expiredAt}
                  onChange={setExpiredAt}
                  placeholder={t('Select date')}
                />
              )}
            </div>
          )}
        </div>
      </div>
    </Dialog>
  )
}

type DataTableBulkActionsProps = {
  table: Table<Redemption>
}

export function DataTableBulkActions(props: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const [deleteTargets, setDeleteTargets] = useState<Redemption[] | null>(null)
  const [editOpen, setEditOpen] = useState(false)
  const selectedRows = props.table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.map((row) => row.original.id)

  const contentToCopy = useMemo(() => {
    const selectedCodes = selectedRows.map((row) => {
      const redemption = row.original
      return `${redemption.name}\t${redemption.key}`
    })
    return selectedCodes.join('\n')
  }, [selectedRows])

  const updateMutation = useMutation({
    mutationFn: async (payload: BatchUpdateRedemptionsPayload) =>
      requireServerSuccess(await batchUpdateRedemptions(payload)),
    onSuccess: (result) => {
      const failed = result.data?.failed ?? []
      const failedIds = new Set(failed.map((item) => String(item.id)))
      props.table.setRowSelection((previous) => {
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
          t('Updated {{updated}} of {{total}} redemption codes', {
            updated: result.data?.updated ?? 0,
            total: selectedRows.length,
          })
        )
        return
      }
      toast.success(
        t('Updated {{count}} redemption codes', {
          count: result.data?.updated ?? 0,
        })
      )
    },
    onError: (error) => handleServerError(error),
  })

  const deletion = useMutation({
    mutationFn: async (targets: Redemption[]) => {
      const result = await batchDeleteRedemptions(
        targets.map((code) => code.id)
      )
      return requireServerSuccess(result)
    },
    onSuccess: (result, targets) => {
      toast.success(
        t('Successfully deleted {{count}} redemption codes', {
          count: result.data ?? 0,
        })
      )
      props.table.setRowSelection((previous) => {
        const next = { ...previous }
        for (const code of targets) delete next[String(code.id)]
        return next
      })
      setDeleteTargets(null)
      triggerRefresh()
    },
    onError: (_error, targets) => {
      handleServerError(
        _error,
        t('Failed to delete {{count}} redemption codes', {
          count: targets.length,
        })
      )
    },
  })

  return (
    <>
      <BulkActionsToolbar table={props.table} entityName={t('redemption code')}>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={() =>
                  updateMutation.mutate({
                    ids: selectedIds,
                    status: REDEMPTION_STATUS.ENABLED,
                  })
                }
                disabled={updateMutation.isPending}
                aria-label={t('Enable selected redemption codes')}
              />
            }
          >
            <Power />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected redemption codes')}</p>
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
                  updateMutation.mutate({
                    ids: selectedIds,
                    status: REDEMPTION_STATUS.DISABLED,
                  })
                }
                disabled={updateMutation.isPending}
                aria-label={t('Disable selected redemption codes')}
              />
            }
          >
            <PowerOff />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected redemption codes')}</p>
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
                disabled={updateMutation.isPending}
                aria-label={t('Batch edit selected redemption codes')}
              />
            }
          >
            <Settings2 />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Batch edit selected redemption codes')}</p>
          </TooltipContent>
        </Tooltip>

        <CopyButton
          value={contentToCopy}
          variant='outline'
          size='icon'
          className='size-8'
          tooltip={t('Copy selected codes')}
          successTooltip={t('Codes copied!')}
          aria-label={t('Copy selected codes')}
        />
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                className='size-8'
                aria-label={t('Delete selected redemption codes')}
                disabled={deletion.isPending}
                onClick={() =>
                  setDeleteTargets(selectedRows.map((row) => row.original))
                }
              />
            }
          >
            <Trash2 aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>
            {t('Delete selected redemption codes')}
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      {editOpen && (
        <RedemptionsBatchEditDialog
          open
          onOpenChange={setEditOpen}
          selectedRows={selectedRows}
          isSubmitting={updateMutation.isPending}
          onSubmit={(payload) => updateMutation.mutate(payload)}
        />
      )}
      <ConfirmDialog
        destructive
        open={deleteTargets !== null}
        onOpenChange={(open) => {
          if (!open && !deletion.isPending) setDeleteTargets(null)
        }}
        title={t('Delete {{count}} redemption codes?', {
          count: deleteTargets?.length ?? 0,
        })}
        desc={t('This action cannot be undone.')}
        confirmText={deletion.isPending ? t('Deleting...') : t('Delete')}
        isLoading={deletion.isPending}
        disabled={!deleteTargets?.length}
        handleConfirm={() => {
          if (deleteTargets?.length && !deletion.isPending) {
            deletion.mutate(deleteTargets)
          }
        }}
      />
    </>
  )
}
