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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { quotaForBillingBasis } from '../billing'
import { buildDashboardFlowData } from '../flow'
import { calculateDashboardStats } from '../stats'

describe('dashboard billing basis', () => {
  test('switches quota totals without changing request or token totals', () => {
    const rows = [
      {
        created_at: 1,
        quota: 240,
        quota_before_group: 100,
        count: 3,
        token_used: 80,
      },
    ]

    assert.deepEqual(calculateDashboardStats(rows, 'charged'), {
      totalQuota: 240,
      totalCount: 3,
      totalTokens: 80,
    })
    assert.deepEqual(calculateDashboardStats(rows, 'before_group'), {
      totalQuota: 100,
      totalCount: 3,
      totalTokens: 80,
    })
  })

  test('treats an omitted base column as zero instead of charged quota', () => {
    assert.equal(quotaForBillingBasis({ quota: 75 }, 'before_group'), 0)
  })

  test('uses the selected quota column throughout channel flow aggregation', () => {
    const rows = [
      {
        user_id: 1,
        username: 'alice',
        use_group: 'vip',
        model_name: 'gpt-test',
        channel_id: 9,
        channel_name: 'cost-channel',
        quota: 240,
        quota_before_group: 100,
        count: 1,
        token_used: 20,
      },
    ]

    const charged = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      billingBasis: 'charged',
    })
    const beforeGroup = buildDashboardFlowData(rows, 'quota', {
      role: 'admin',
      billingBasis: 'before_group',
    })

    assert.equal(charged.summary.quota, 240)
    assert.equal(beforeGroup.summary.quota, 100)
    assert.equal(
      beforeGroup.flow.links.find((link) => link.target === 'channel:9')?.value,
      100
    )
    assert.equal(
      beforeGroup.filterOptions.nodes.find(
        (option) => option.value === 'channel:9'
      )?.valueRaw,
      100
    )
  })
})
