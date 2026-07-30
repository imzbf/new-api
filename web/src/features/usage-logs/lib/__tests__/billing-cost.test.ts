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

import type { LogOtherData } from '../../types'
import {
  calculateBillingCostBreakdown,
  type BillingCostKind,
} from '../billing-cost'

function getCost(
  lines: ReturnType<typeof calculateBillingCostBreakdown>,
  kind: BillingCostKind
): number {
  const line = lines.find((item) => item.kind === kind)
  assert.ok(line, `missing ${kind} cost line`)
  return line.costUSD
}

function assertClose(actual: number, expected: number): void {
  assert.ok(
    Math.abs(actual - expected) < 1e-12,
    `expected ${actual} to equal ${expected}`
  )
}

describe('usage log billing cost breakdown', () => {
  test('calculates standard input and output costs with the effective group ratio', () => {
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 1000, completion_tokens: 500 },
      {
        model_ratio: 1,
        completion_ratio: 2,
        group_ratio: 1.5,
        user_group_ratio: -1,
      }
    )

    assertClose(getCost(lines, 'input'), 0.003)
    assertClose(getCost(lines, 'output'), 0.003)
  })

  test('separates OpenAI cache reads from the input tokens that include them', () => {
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 1000, completion_tokens: 0 },
      {
        model_ratio: 1,
        completion_ratio: 1,
        group_ratio: 2,
        cache_tokens: 400,
        cache_ratio: 0.1,
        usage_semantic: 'openai',
      }
    )

    assert.deepEqual(lines.find((line) => line.kind === 'input')?.terms, [
      { tokens: 600, unitPriceUSD: 2 },
    ])
    assertClose(getCost(lines, 'input'), 0.0024)
    assertClose(getCost(lines, 'cache_read'), 0.00016)
  })

  test('keeps Anthropic input tokens separate from cache read and timed writes', () => {
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 1000, completion_tokens: 100 },
      {
        model_ratio: 1,
        completion_ratio: 2,
        group_ratio: 1,
        cache_tokens: 200,
        cache_ratio: 0.1,
        cache_creation_tokens: 300,
        cache_creation_tokens_5m: 200,
        cache_creation_tokens_1h: 100,
        cache_creation_ratio: 1.25,
        cache_creation_ratio_5m: 1.25,
        cache_creation_ratio_1h: 2,
        usage_semantic: 'anthropic',
      }
    )

    assert.deepEqual(lines.find((line) => line.kind === 'input')?.terms, [
      { tokens: 1000, unitPriceUSD: 2 },
    ])
    assertClose(getCost(lines, 'cache_read'), 0.00004)
    assertClose(getCost(lines, 'cache_write_5m'), 0.0005)
    assertClose(getCost(lines, 'cache_write_1h'), 0.0004)
  })

  test('clamps overlapping cache prefixes instead of showing a negative input fee', () => {
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 100, completion_tokens: 0 },
      {
        model_ratio: 1,
        completion_ratio: 1,
        group_ratio: 1,
        cache_tokens: 80,
        cache_ratio: 0.1,
        cache_creation_tokens: 60,
        cache_creation_ratio: 1.25,
        usage_semantic: 'openai',
      }
    )

    assert.equal(
      lines.find((line) => line.kind === 'input')?.terms[0].tokens,
      0
    )
    assert.equal(getCost(lines, 'input'), 0)
    assertClose(getCost(lines, 'cache_read'), 0.000016)
    assertClose(getCost(lines, 'cache_write'), 0.00015)
  })

  test('does not count Anthropic one-hour cache writes again as five-minute writes', () => {
    const expr = 'tier("base", p * 2 + c * 8 + cc * 2.5 + cc1h * 4)'
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 500, completion_tokens: 0 },
      {
        billing_mode: 'tiered_expr',
        expr_b64: Buffer.from(expr, 'utf8').toString('base64'),
        matched_tier: 'base',
        group_ratio: 1,
        cache_creation_tokens: 100,
        cache_creation_tokens_1h: 100,
        usage_semantic: 'anthropic',
      }
    )

    assert.equal(
      lines.some((line) => line.kind === 'cache_write_5m'),
      false
    )
    assertClose(getCost(lines, 'cache_write_1h'), 0.0004)
  })

  test('uses matched tier prices and only excludes cache when separately priced', () => {
    const expr = 'tier("base", p * 3 + c * 15 + cr * 0.3)'
    const other: LogOtherData = {
      billing_mode: 'tiered_expr',
      expr_b64: Buffer.from(expr, 'utf8').toString('base64'),
      matched_tier: 'base',
      group_ratio: 2,
      cache_tokens: 400,
      usage_semantic: 'openai',
    }
    const lines = calculateBillingCostBreakdown(
      { prompt_tokens: 1000, completion_tokens: 100 },
      other
    )

    assert.deepEqual(lines.find((line) => line.kind === 'input')?.terms, [
      { tokens: 600, unitPriceUSD: 3 },
    ])
    assertClose(getCost(lines, 'input'), 0.0036)
    assertClose(getCost(lines, 'output'), 0.003)
    assertClose(getCost(lines, 'cache_read'), 0.00024)
  })

  test('keeps zero-token input and output rows but skips unavailable pricing', () => {
    const freeLines = calculateBillingCostBreakdown(
      { prompt_tokens: 0, completion_tokens: 0 },
      {
        model_ratio: 1,
        completion_ratio: 1,
        group_ratio: 1,
      }
    )
    assert.deepEqual(
      freeLines.map((line) => line.kind),
      ['input', 'output']
    )
    assert.equal(getCost(freeLines, 'input'), 0)
    assert.equal(getCost(freeLines, 'output'), 0)

    assert.deepEqual(
      calculateBillingCostBreakdown(
        { prompt_tokens: 100, completion_tokens: 50 },
        { group_ratio: 1 }
      ),
      []
    )
  })
})
