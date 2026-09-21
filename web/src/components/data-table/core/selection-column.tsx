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
import type {
  ColumnDef,
  Row,
  RowSelectionState,
  Table,
} from '@tanstack/react-table'
import { type MouseEvent, useCallback, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'

export function invertSelectionRange(
  rowIds: string[],
  selection: RowSelectionState,
  anchorId: string,
  currentId: string
): RowSelectionState {
  const next = { ...selection }
  const anchorIndex = rowIds.indexOf(anchorId)
  const currentIndex = rowIds.indexOf(currentId)

  if (
    anchorIndex === -1 ||
    currentIndex === -1 ||
    anchorIndex === currentIndex
  ) {
    next[currentId] = !selection[currentId]
    return next
  }

  const start = Math.min(anchorIndex, currentIndex)
  const end = Math.max(anchorIndex, currentIndex)
  for (let index = start; index <= end; index += 1) {
    if (index === anchorIndex) continue
    const rowId = rowIds[index]
    next[rowId] = !selection[rowId]
  }
  return next
}

export function useSelectionColumn<TData>(): ColumnDef<TData> {
  const { t } = useTranslation()
  const lastSelectedRowIdRef = useRef<string | null>(null)
  const skipNextCheckedChangeRef = useRef(false)

  const handleShiftClick = useCallback(
    (event: MouseEvent<HTMLElement>, table: Table<TData>, row: Row<TData>) => {
      if (!event.shiftKey) {
        lastSelectedRowIdRef.current = row.id
        return
      }
      event.preventDefault()
      event.stopPropagation()
      skipNextCheckedChangeRef.current = true
      window.setTimeout(() => {
        skipNextCheckedChangeRef.current = false
      }, 0)

      const rowIds = table.getRowModel().rows.map((item) => item.id)
      const anchorId = lastSelectedRowIdRef.current ?? row.id
      table.setRowSelection((selection) =>
        invertSelectionRange(rowIds, selection, anchorId, row.id)
      )
      lastSelectedRowIdRef.current = row.id
    },
    []
  )

  return useMemo(
    () => ({
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => {
            lastSelectedRowIdRef.current = null
            table.toggleAllPageRowsSelected(!!value)
          }}
          aria-label={t('Select all')}
          className='translate-y-[2px]'
        />
      ),
      cell: ({ row, table }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onClickCapture={(event) => handleShiftClick(event, table, row)}
          onCheckedChange={(value) => {
            if (skipNextCheckedChangeRef.current) {
              skipNextCheckedChangeRef.current = false
              return
            }
            lastSelectedRowIdRef.current = row.id
            row.toggleSelected(!!value)
          }}
          aria-label={t('Select row')}
          className='translate-y-[2px]'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    }),
    [handleShiftClick, t]
  )
}
