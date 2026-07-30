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

import { buildCommonLogSearchKey } from '../filter'

describe('common log filter identity', () => {
  test('treats equivalent date instances as the same effective filter state', () => {
    const first = {
      startTime: new Date('2026-07-01T00:00:00.000Z'),
      endTime: new Date('2026-07-30T23:59:59.999Z'),
      model: 'gpt-4o',
    }
    const second = {
      ...first,
      startTime: new Date(first.startTime),
      endTime: new Date(first.endTime),
    }

    assert.equal(
      buildCommonLogSearchKey(first, '0'),
      buildCommonLogSearchKey(second, '0')
    )
  })

  test('changes when a filter or log type changes', () => {
    const filters = {
      model: 'gpt-4o',
      group: 'default',
    }
    const baseKey = buildCommonLogSearchKey(filters, '0')

    assert.notEqual(
      baseKey,
      buildCommonLogSearchKey({ ...filters, model: 'claude-3' }, '0')
    )
    assert.notEqual(baseKey, buildCommonLogSearchKey(filters, '2'))
  })
})
