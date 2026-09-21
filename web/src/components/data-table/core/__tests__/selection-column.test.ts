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
import type { RowSelectionState } from '@tanstack/react-table'
import { describe, expect, it } from 'vitest'

import { invertSelectionRange } from '../selection-column'

const rowIds = ['a', 'b', 'c', 'd']

function invert(
  selection: RowSelectionState,
  anchorId: string,
  currentId: string
) {
  return invertSelectionRange(rowIds, selection, anchorId, currentId)
}

describe('invertSelectionRange', () => {
  it('inverts the forward range without changing the anchor row', () => {
    expect(invert({ a: true, b: true }, 'a', 'd')).toEqual({
      a: true,
      b: false,
      c: true,
      d: true,
    })
  })

  it('inverts the backward range without changing the anchor row', () => {
    expect(invert({ b: true, d: true }, 'd', 'b')).toEqual({
      b: false,
      c: true,
      d: true,
    })
  })

  it('inverts the current row when the anchor is the same row', () => {
    expect(invert({ b: true }, 'b', 'b')).toEqual({ b: false })
    expect(invert({}, 'b', 'b')).toEqual({ b: true })
  })

  it('falls back to the current row when the anchor is not visible', () => {
    expect(invert({ b: true }, 'missing', 'c')).toEqual({
      b: true,
      c: true,
    })
  })
})
