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
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { decodeBillingExprB64, getTieredBillingSummary } from './format'

const TOKENS_PER_MILLION = 1_000_000

export type BillingCostKind =
  | 'input'
  | 'output'
  | 'cache_read'
  | 'cache_write'
  | 'cache_write_5m'
  | 'cache_write_1h'

export interface BillingCostTerm {
  tokens: number
  unitPriceUSD: number
}

export interface BillingCostLine {
  kind: BillingCostKind
  terms: BillingCostTerm[]
  groupRatio: number
  costUSD: number
}

type UsageTokenCounts = Pick<UsageLog, 'prompt_tokens' | 'completion_tokens'>

function toNonNegativeNumber(value: number | null | undefined): number {
  if (value == null || !Number.isFinite(value) || value <= 0) return 0
  return value
}

function toUnitPrice(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return null
  }
  return value
}

/**
 * Resolve the exact group multiplier used by settlement. A user-exclusive
 * ratio is already copied into group_ratio by the backend, but preferring the
 * explicit field keeps historical logs and the existing UI semantics aligned.
 */
export function getEffectiveBillingGroupRatio(other: LogOtherData): number {
  const userRatio = other.user_group_ratio
  if (userRatio != null && Number.isFinite(userRatio) && userRatio >= 0) {
    return userRatio
  }

  const groupRatio = other.group_ratio
  if (groupRatio != null && Number.isFinite(groupRatio) && groupRatio >= 0) {
    return groupRatio
  }

  return 1
}

function isAnthropicUsage(other: LogOtherData): boolean {
  if (other.usage_semantic) {
    return other.usage_semantic.toLowerCase() === 'anthropic'
  }

  // Older logs predate usage_semantic. For those entries, claude is the best
  // available signal that prompt_tokens excludes cache read/write tokens.
  return other.claude === true
}

function getCacheWriteCounts(other: LogOtherData): {
  total: number
  generic: number
  fiveMinutes: number
  oneHour: number
} {
  const reportedTotal = toNonNegativeNumber(other.cache_creation_tokens)
  const fiveMinutes = toNonNegativeNumber(other.cache_creation_tokens_5m)
  const oneHour = toNonNegativeNumber(other.cache_creation_tokens_1h)
  const splitTotal = fiveMinutes + oneHour
  const total = Math.max(reportedTotal, splitTotal)

  return {
    total,
    generic: Math.max(0, total - splitTotal),
    fiveMinutes,
    oneHour,
  }
}

function createCostLine(
  kind: BillingCostKind,
  terms: BillingCostTerm[],
  groupRatio: number
): BillingCostLine | null {
  if (terms.length === 0) return null

  const costUSD =
    terms.reduce((sum, term) => sum + term.tokens * term.unitPriceUSD, 0) *
    (groupRatio / TOKENS_PER_MILLION)

  return { kind, terms, groupRatio, costUSD }
}

function calculateStandardBillingCosts(
  log: UsageTokenCounts,
  other: LogOtherData,
  groupRatio: number
): BillingCostLine[] {
  const modelRatio = toUnitPrice(other.model_ratio)
  if (modelRatio == null) return []

  const baseInputPrice = modelRatio * 2
  const completionRatio = toUnitPrice(other.completion_ratio)
  const cacheRatio = toUnitPrice(other.cache_ratio)
  const cacheCreationRatio = toUnitPrice(other.cache_creation_ratio)
  const cacheCreationRatio5m =
    toUnitPrice(other.cache_creation_ratio_5m) ?? cacheCreationRatio
  const cacheCreationRatio1h = toUnitPrice(other.cache_creation_ratio_1h)

  const promptTokens = toNonNegativeNumber(log.prompt_tokens)
  const completionTokens = toNonNegativeNumber(log.completion_tokens)
  const cacheReadTokens = toNonNegativeNumber(other.cache_tokens)
  const cacheWrite = getCacheWriteCounts(other)
  const inputTerms: BillingCostTerm[] = []
  const outputTerms: BillingCostTerm[] = []

  if (other.audio || other.ws) {
    const audioInputTokens = toNonNegativeNumber(other.audio_input)
    const audioOutputTokens = toNonNegativeNumber(other.audio_output)
    const audioRatio = toUnitPrice(other.audio_ratio)
    const audioCompletionRatio = toUnitPrice(other.audio_completion_ratio)
    const hasAudioInputPrice = audioInputTokens > 0 && audioRatio != null
    const hasAudioOutputPrice =
      audioOutputTokens > 0 &&
      audioRatio != null &&
      audioCompletionRatio != null

    const textInputTokens = hasAudioInputPrice
      ? toNonNegativeNumber(other.text_input) ||
        Math.max(0, promptTokens - audioInputTokens)
      : promptTokens
    const textOutputTokens = hasAudioOutputPrice
      ? toNonNegativeNumber(other.text_output) ||
        Math.max(0, completionTokens - audioOutputTokens)
      : completionTokens

    inputTerms.push({ tokens: textInputTokens, unitPriceUSD: baseInputPrice })
    if (hasAudioInputPrice) {
      inputTerms.push({
        tokens: audioInputTokens,
        unitPriceUSD: baseInputPrice * audioRatio,
      })
    }

    if (completionRatio != null) {
      outputTerms.push({
        tokens: textOutputTokens,
        unitPriceUSD: baseInputPrice * completionRatio,
      })
      if (hasAudioOutputPrice) {
        outputTerms.push({
          tokens: audioOutputTokens,
          unitPriceUSD: baseInputPrice * audioRatio * audioCompletionRatio,
        })
      }
    }
  } else {
    let baseInputTokens = promptTokens
    if (!isAnthropicUsage(other)) {
      baseInputTokens -= cacheReadTokens + cacheWrite.total
    }

    const imageTokens = other.image
      ? toNonNegativeNumber(other.image_output)
      : 0
    const imageRatio = toUnitPrice(other.image_ratio)
    if (imageTokens > 0 && imageRatio != null) {
      baseInputTokens -= imageTokens
      inputTerms.push({
        tokens: imageTokens,
        unitPriceUSD: baseInputPrice * imageRatio,
      })
    }

    const separatelyPricedAudioTokens = other.audio_input_seperate_price
      ? toNonNegativeNumber(other.audio_input_token_count)
      : 0
    const separateAudioInputPrice = toUnitPrice(other.audio_input_price)
    if (separatelyPricedAudioTokens > 0 && separateAudioInputPrice != null) {
      baseInputTokens -= separatelyPricedAudioTokens
      inputTerms.push({
        tokens: separatelyPricedAudioTokens,
        unitPriceUSD: separateAudioInputPrice,
      })
    }

    // Cache-write prefixes can overlap cache-read prefixes and exceed the
    // reported prompt total. The backend clamps this same remainder at zero so
    // the calculation shown in the log can never imply a negative input fee.
    inputTerms.unshift({
      tokens: Math.max(0, baseInputTokens),
      unitPriceUSD: baseInputPrice,
    })

    if (completionRatio != null) {
      outputTerms.push({
        tokens: completionTokens,
        unitPriceUSD: baseInputPrice * completionRatio,
      })
    }
  }

  const lines: BillingCostLine[] = []
  const inputLine = createCostLine('input', inputTerms, groupRatio)
  const outputLine = createCostLine('output', outputTerms, groupRatio)
  if (inputLine) lines.push(inputLine)
  if (outputLine) lines.push(outputLine)

  if (cacheReadTokens > 0 && cacheRatio != null) {
    const line = createCostLine(
      'cache_read',
      [
        {
          tokens: cacheReadTokens,
          unitPriceUSD: baseInputPrice * cacheRatio,
        },
      ],
      groupRatio
    )
    if (line) lines.push(line)
  }

  if (cacheWrite.generic > 0 && cacheCreationRatio != null) {
    const line = createCostLine(
      'cache_write',
      [
        {
          tokens: cacheWrite.generic,
          unitPriceUSD: baseInputPrice * cacheCreationRatio,
        },
      ],
      groupRatio
    )
    if (line) lines.push(line)
  }

  if (cacheWrite.fiveMinutes > 0 && cacheCreationRatio5m != null) {
    const line = createCostLine(
      'cache_write_5m',
      [
        {
          tokens: cacheWrite.fiveMinutes,
          unitPriceUSD: baseInputPrice * cacheCreationRatio5m,
        },
      ],
      groupRatio
    )
    if (line) lines.push(line)
  }

  if (cacheWrite.oneHour > 0 && cacheCreationRatio1h != null) {
    const line = createCostLine(
      'cache_write_1h',
      [
        {
          tokens: cacheWrite.oneHour,
          unitPriceUSD: baseInputPrice * cacheCreationRatio1h,
        },
      ],
      groupRatio
    )
    if (line) lines.push(line)
  }

  return lines
}

function getTieredUsedVars(expr: string): Set<string> {
  const usedVars = new Set<string>()
  const variablePattern = /\b(p|c|cr|cc|cc1h|img|img_o|ai|ao)\b/g
  for (const match of expr.matchAll(variablePattern)) {
    usedVars.add(match[1])
  }
  return usedVars
}

function calculateTieredBillingCosts(
  log: UsageTokenCounts,
  other: LogOtherData,
  groupRatio: number
): BillingCostLine[] {
  const summary = getTieredBillingSummary(other)
  if (!summary) return []

  const expr = decodeBillingExprB64(other.expr_b64)
  const usedVars = getTieredUsedVars(expr)
  const promptTokens = toNonNegativeNumber(log.prompt_tokens)
  const completionTokens = toNonNegativeNumber(log.completion_tokens)
  const cacheReadTokens = toNonNegativeNumber(other.cache_tokens)
  const cacheWrite = getCacheWriteCounts(other)
  const anthropicUsage = isAnthropicUsage(other)

  const hasTimedCacheWriteSplit =
    cacheWrite.fiveMinutes > 0 || cacheWrite.oneHour > 0
  let cacheWriteTokens = cacheWrite.total
  if (anthropicUsage && hasTimedCacheWriteSplit) {
    cacheWriteTokens = cacheWrite.fiveMinutes
  }
  const cacheWrite1hTokens = anthropicUsage ? cacheWrite.oneHour : 0
  const imageInputTokens = other.image
    ? toNonNegativeNumber(other.image_output)
    : 0
  const audioInputTokens = toNonNegativeNumber(
    other.audio_input_token_count ?? other.audio_input
  )
  const audioOutputTokens = toNonNegativeNumber(other.audio_output)

  let inputTokens = promptTokens
  let outputTokens = completionTokens
  if (!anthropicUsage) {
    if (usedVars.has('cr')) inputTokens -= cacheReadTokens
    if (usedVars.has('cc')) inputTokens -= cacheWriteTokens
    if (usedVars.has('cc1h')) inputTokens -= cacheWrite1hTokens
    if (usedVars.has('img')) inputTokens -= imageInputTokens
    if (usedVars.has('ai')) inputTokens -= audioInputTokens
    if (usedVars.has('ao')) outputTokens -= audioOutputTokens
  }

  const inputTerms: BillingCostTerm[] = []
  const outputTerms: BillingCostTerm[] = []
  const inputPrice = toUnitPrice(summary.tier.inputPrice)
  const outputPrice = toUnitPrice(summary.tier.outputPrice)
  const imagePrice = toUnitPrice(summary.tier.imagePrice)
  const audioInputPrice = toUnitPrice(summary.tier.audioInputPrice)
  const audioOutputPrice = toUnitPrice(summary.tier.audioOutputPrice)

  if (usedVars.has('p') && inputPrice != null) {
    inputTerms.push({
      tokens: Math.max(0, inputTokens),
      unitPriceUSD: inputPrice,
    })
  }
  if (usedVars.has('img') && imageInputTokens > 0 && imagePrice != null) {
    inputTerms.push({ tokens: imageInputTokens, unitPriceUSD: imagePrice })
  }
  if (usedVars.has('ai') && audioInputTokens > 0 && audioInputPrice != null) {
    inputTerms.push({ tokens: audioInputTokens, unitPriceUSD: audioInputPrice })
  }

  if (usedVars.has('c') && outputPrice != null) {
    outputTerms.push({
      tokens: Math.max(0, outputTokens),
      unitPriceUSD: outputPrice,
    })
  }
  if (usedVars.has('ao') && audioOutputTokens > 0 && audioOutputPrice != null) {
    outputTerms.push({
      tokens: audioOutputTokens,
      unitPriceUSD: audioOutputPrice,
    })
  }

  const lines: BillingCostLine[] = []
  const inputLine = createCostLine('input', inputTerms, groupRatio)
  const outputLine = createCostLine('output', outputTerms, groupRatio)
  if (inputLine) lines.push(inputLine)
  if (outputLine) lines.push(outputLine)

  const cacheReadPrice = toUnitPrice(summary.tier.cacheReadPrice)
  if (usedVars.has('cr') && cacheReadTokens > 0 && cacheReadPrice != null) {
    const line = createCostLine(
      'cache_read',
      [{ tokens: cacheReadTokens, unitPriceUSD: cacheReadPrice }],
      groupRatio
    )
    if (line) lines.push(line)
  }

  const cacheWritePrice = toUnitPrice(summary.tier.cacheCreatePrice)
  if (usedVars.has('cc') && cacheWriteTokens > 0 && cacheWritePrice != null) {
    const kind =
      anthropicUsage && cacheWrite.fiveMinutes > 0
        ? 'cache_write_5m'
        : 'cache_write'
    const line = createCostLine(
      kind,
      [{ tokens: cacheWriteTokens, unitPriceUSD: cacheWritePrice }],
      groupRatio
    )
    if (line) lines.push(line)
  }

  const cacheWrite1hPrice = toUnitPrice(summary.tier.cacheCreate1hPrice)
  if (
    usedVars.has('cc1h') &&
    cacheWrite1hTokens > 0 &&
    cacheWrite1hPrice != null
  ) {
    const line = createCostLine(
      'cache_write_1h',
      [{ tokens: cacheWrite1hTokens, unitPriceUSD: cacheWrite1hPrice }],
      groupRatio
    )
    if (line) lines.push(line)
  }

  return lines
}

/**
 * Reconstruct the input, output, and cache portions from the immutable values
 * stored with a usage log. The total log quota remains authoritative because
 * it can also include rounding, tool surcharges, or provider-specific extras.
 */
export function calculateBillingCostBreakdown(
  log: UsageTokenCounts,
  other: LogOtherData
): BillingCostLine[] {
  if ((other.model_price ?? 0) > 0) return []

  const groupRatio = getEffectiveBillingGroupRatio(other)
  if (other.billing_mode === 'tiered_expr') {
    return calculateTieredBillingCosts(log, other, groupRatio)
  }

  return calculateStandardBillingCosts(log, other, groupRatio)
}
