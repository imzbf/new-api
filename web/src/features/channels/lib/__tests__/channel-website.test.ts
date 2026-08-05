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

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

function channelForm(website: string) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Example channel',
    website,
    key: 'test-key',
    models: 'gpt-5',
  }
}

describe('channel website form', () => {
  test('accepts only blank or HTTP(S) website addresses', () => {
    assert.equal(channelFormSchema.safeParse(channelForm('')).success, true)
    assert.equal(
      channelFormSchema.safeParse(channelForm('http://example.com')).success,
      true
    )
    assert.equal(
      channelFormSchema.safeParse(channelForm('https://example.com/docs'))
        .success,
      true
    )
    assert.equal(
      channelFormSchema.safeParse(channelForm('example.com')).success,
      false
    )
    assert.equal(
      channelFormSchema.safeParse(channelForm('javascript:alert(1)')).success,
      false
    )
    assert.equal(
      channelFormSchema.safeParse(
        channelForm(`https://example.com/${'a'.repeat(2049)}`)
      ).success,
      false
    )
  })

  test('trims website addresses in create and update payloads', () => {
    const formData = channelForm('  https://example.com/docs  ')

    assert.equal(
      transformFormDataToCreatePayload(formData).channel.website,
      'https://example.com/docs'
    )
    assert.equal(
      transformFormDataToUpdatePayload(formData, 12).website,
      'https://example.com/docs'
    )
  })

  test('sends an explicit empty website when clearing an existing channel', () => {
    assert.equal(
      transformFormDataToUpdatePayload(channelForm(''), 12).website,
      ''
    )
  })
})
