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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ChannelWebsiteLink } = await import('../channel-website-link')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Open {{name}} official website': 'Open {{name}} official website',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

async function renderWebsiteLink(website: string) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ChannelWebsiteLink channelName='Example' website={website} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

describe('channel website link', () => {
  after(() => {
    domWindow.close()
  })

  test('opens a valid website in an isolated new window', async () => {
    const rendered = await renderWebsiteLink('https://example.com/docs')
    const link = rendered.container.querySelector('a')

    assert.ok(link)
    assert.equal(link.getAttribute('href'), 'https://example.com/docs')
    assert.equal(link.getAttribute('target'), '_blank')
    assert.equal(link.getAttribute('rel'), 'noopener noreferrer')
    assert.equal(
      link.getAttribute('aria-label'),
      'Open Example official website'
    )

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })

  test('does not render a link for an unsafe protocol', async () => {
    const rendered = await renderWebsiteLink('javascript:alert(1)')

    assert.equal(rendered.container.querySelector('a'), null)

    await act(async () => rendered.root.unmount())
    rendered.container.remove()
  })
})
