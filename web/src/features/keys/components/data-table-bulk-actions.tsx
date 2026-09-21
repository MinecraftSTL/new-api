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
import { Copy, Loader2, Power, PowerOff, Settings2, Trash2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
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
import { getUserGroups, getUserModels } from '@/lib/api'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import {
  formatQuota,
  formatTimestampToDate,
  parseQuotaFromDollars,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { batchDeleteApiKeys, batchUpdateApiKeys } from '../api'
import { API_KEY_STATUS } from '../constants'
import type { ApiKey, BatchUpdateApiKeysPayload } from '../types'
import { ApiKeysMultiDeleteDialog } from './api-keys-multi-delete-dialog'
import { useApiKeys } from './api-keys-provider'

type ApiKeyBatchField = 'status' | 'quota' | 'group' | 'expiry' | 'models'

type ApiKeysBatchEditDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  selectedRows: Row<ApiKey>[]
  isSubmitting: boolean
  onSubmit: (payload: BatchUpdateApiKeysPayload) => void
}

function ApiKeysBatchEditDialog({
  open,
  onOpenChange,
  selectedRows,
  isSubmitting,
  onSubmit,
}: ApiKeysBatchEditDialogProps) {
  const { t } = useTranslation()
  const [sourceId, setSourceId] = useState('manual')
  const [fields, setFields] = useState<Record<ApiKeyBatchField, boolean>>({
    status: false,
    quota: false,
    group: false,
    expiry: false,
    models: false,
  })
  const [status, setStatus] = useState(String(API_KEY_STATUS.ENABLED))
  const [unlimitedQuota, setUnlimitedQuota] = useState(false)
  const [quotaAmount, setQuotaAmount] = useState('')
  const [group, setGroup] = useState('')
  const [expiresNever, setExpiresNever] = useState(true)
  const [expiredAt, setExpiredAt] = useState<Date | undefined>()
  const [modelLimits, setModelLimits] = useState<string[]>([])

  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: async () => requireServerSuccess(await getUserGroups()),
    enabled: open,
    staleTime: 0,
  })
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: async () => requireServerSuccess(await getUserModels()),
    enabled: open,
    staleTime: 0,
  })
  const groups = Object.keys(groupsData?.data ?? {})
  const models = modelsData?.data ?? []
  const sourceRow = selectedRows.find(
    (row) => String(row.original.id) === sourceId
  )
  const source = sourceRow?.original
  const availableGroups = [
    ...new Set<string>([...groups, source?.group ?? '', group].filter(Boolean)),
  ]

  const toggleField = (field: ApiKeyBatchField, checked: boolean) => {
    setFields((previous) => ({ ...previous, [field]: checked }))
  }

  const handleSubmit = () => {
    const selectedFields = Object.values(fields).some(Boolean)
    if (!selectedFields) {
      toast.warning(t('Select at least one field to update'))
      return
    }
    const payload: BatchUpdateApiKeysPayload = {
      ids: selectedRows.map((row) => row.original.id),
    }
    if (fields.status) {
      payload.status = Number(resolvedStatus)
    }
    if (fields.quota) {
      const resolvedUnlimited = source ? source.unlimited_quota : unlimitedQuota
      const parsedQuota = Number(quotaAmount || 0)
      if (
        !resolvedUnlimited &&
        (!Number.isFinite(parsedQuota) || parsedQuota < 0)
      ) {
        toast.error(t('Enter a valid quota amount'))
        return
      }
      payload.unlimited_quota = resolvedUnlimited
      payload.remain_quota = source
        ? source.remain_quota
        : parseQuotaFromDollars(parsedQuota)
    }
    if (fields.group) {
      payload.group = source
        ? (source.group ?? '')
        : group || availableGroups[0]
    }
    if (fields.expiry) {
      if (source) {
        payload.expired_time = source.expired_time
      } else if (expiresNever) {
        payload.expired_time = -1
      } else if (expiredAt) {
        payload.expired_time = Math.floor(expiredAt.getTime() / 1000)
      } else {
        toast.error(t('Select an expiration time'))
        return
      }
    }
    if (fields.models) {
      const limits = source
        ? (source.model_limits || '').split(',').filter(Boolean)
        : modelLimits
      payload.model_limits_enabled = limits.length > 0
      payload.model_limits = limits.join(',')
    }
    onSubmit(payload)
  }

  let resolvedStatus = status
  if (source) {
    resolvedStatus =
      source.status === API_KEY_STATUS.ENABLED
        ? String(API_KEY_STATUS.ENABLED)
        : String(API_KEY_STATUS.DISABLED)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Batch edit API keys')}
      description={t(
        'Choose the fields to overwrite for {{count}} selected API keys.',
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
              'Choose Manual values or copy selected fields from one selected API key.'
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
                value == null ? String(API_KEY_STATUS.ENABLED) : String(value)
              )
            }
            disabled={Boolean(source)}
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectItem value={String(API_KEY_STATUS.ENABLED)}>
                {t('Enabled')}
              </SelectItem>
              <SelectItem value={String(API_KEY_STATUS.DISABLED)}>
                {t('Disabled')}
              </SelectItem>
            </SelectContent>
          </Select>
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
            <Input
              value={
                source.unlimited_quota
                  ? t('Unlimited')
                  : formatQuota(source.remain_quota)
              }
              disabled
              readOnly
            />
          ) : (
            <div className='space-y-3'>
              <label className='flex items-center justify-between gap-3 text-sm'>
                <span>{t('Unlimited Quota')}</span>
                <Switch
                  checked={unlimitedQuota}
                  onCheckedChange={setUnlimitedQuota}
                />
              </label>
              {!unlimitedQuota && (
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
          )}
        </div>

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.group}
              onCheckedChange={(value) => toggleField('group', !!value)}
            />
            {t('Group')}
          </label>
          <Select
            value={source ? (source.group ?? '') : group || availableGroups[0]}
            onValueChange={(value) => setGroup(value ?? '')}
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
                source.expired_time === -1
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

        <div className='space-y-3 rounded-md border p-3'>
          <label className='flex items-center gap-2 text-sm font-medium'>
            <Checkbox
              checked={fields.models}
              onCheckedChange={(value) => toggleField('models', !!value)}
            />
            {t('Model Limits')}
          </label>
          {source ? (
            <Input
              value={
                source.model_limits_enabled
                  ? (source.model_limits ?? '')
                  : t('Allow all models')
              }
              disabled
              readOnly
            />
          ) : (
            <MultiSelect
              options={models.map((model) => ({ label: model, value: model }))}
              selected={modelLimits}
              onChange={setModelLimits}
              placeholder={t('Select models (empty for allow all)')}
            />
          )}
        </div>
      </div>
    </Dialog>
  )
}

type DataTableBulkActionsProps = {
  table: Table<ApiKey>
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { resolveRealKeysBatch, triggerRefresh } = useApiKeys()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [isCopying, setIsCopying] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedIds = selectedRows.map((row) => row.original.id)

  const updateMutation = useMutation({
    mutationFn: async (payload: BatchUpdateApiKeysPayload) =>
      requireServerSuccess(await batchUpdateApiKeys(payload)),
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
          t('Updated {{updated}} of {{total}} API keys', {
            updated: result.data?.updated ?? 0,
            total: selectedRows.length,
          })
        )
        return
      }
      toast.success(
        t('Updated {{count}} API keys', {
          count: result.data?.updated ?? 0,
        })
      )
    },
    onError: (error) => handleServerError(error),
  })

  const handleBatchCopy = useCallback(async () => {
    if (selectedRows.length === 0) return
    setIsCopying(true)
    try {
      const keysMap = await resolveRealKeysBatch(selectedIds)
      const lines: string[] = []
      for (const row of selectedRows) {
        const apiKey = row.original
        const realKey = keysMap[apiKey.id]
        if (realKey) lines.push(`${apiKey.name}\t${realKey}`)
      }
      if (lines.length > 0) {
        const ok = await copyToClipboard(lines.join('\n'))
        if (ok) {
          toast.success(t('Copied {{count}} key(s)', { count: lines.length }))
        } else {
          toast.error(t('Failed to copy keys'))
        }
      }
    } catch {
      toast.error(t('Failed to copy keys'))
    } finally {
      setIsCopying(false)
    }
  }, [resolveRealKeysBatch, selectedIds, selectedRows, t])

  const deletion = useMutation({
    mutationFn: async () => {
      const result = await batchDeleteApiKeys(selectedIds)
      return requireServerSuccess(result)
    },
    onSuccess: () => {
      toast.success(
        t('Deleted {{count}} API keys', { count: selectedIds.length })
      )
      table.resetRowSelection()
      setShowDeleteConfirm(false)
      triggerRefresh()
    },
    onError: (error) => handleServerError(error),
  })

  return (
    <>
      <BulkActionsToolbar table={table} entityName='API key'>
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
                    status: API_KEY_STATUS.ENABLED,
                  })
                }
                disabled={updateMutation.isPending}
                aria-label={t('Enable selected API keys')}
              />
            }
          >
            <Power />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected API keys')}</p>
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
                    status: API_KEY_STATUS.DISABLED,
                  })
                }
                disabled={updateMutation.isPending}
                aria-label={t('Disable selected API keys')}
              />
            }
          >
            <PowerOff />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected API keys')}</p>
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
                aria-label={t('Batch edit selected API keys')}
              />
            }
          >
            <Settings2 />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Batch edit selected API keys')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                onClick={handleBatchCopy}
                disabled={isCopying}
                aria-label={t('Copy selected keys')}
              />
            }
          >
            {isCopying ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <Copy className='size-4' />
            )}
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Copy selected keys')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => setShowDeleteConfirm(true)}
                className='size-8'
                aria-label={t('Delete selected API keys')}
                disabled={deletion.isPending}
              />
            }
          >
            <Trash2 />
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete selected API keys')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      {editOpen && (
        <ApiKeysBatchEditDialog
          open
          onOpenChange={setEditOpen}
          selectedRows={selectedRows}
          isSubmitting={updateMutation.isPending}
          onSubmit={(payload) => updateMutation.mutate(payload)}
        />
      )}
      <ApiKeysMultiDeleteDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        table={table}
      />
    </>
  )
}
