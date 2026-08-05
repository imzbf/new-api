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
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

type ChannelWebsiteLinkProps = {
  channelName: string
  website: string | null | undefined
}

export function ChannelWebsiteLink(props: ChannelWebsiteLinkProps) {
  const { t } = useTranslation()
  const website = props.website?.trim()
  if (!website) return null

  // The API validates this field too, but keep the rendered link inert if
  // legacy or manually edited database content contains an unsafe protocol.
  try {
    const parsedURL = new URL(website)
    if (parsedURL.protocol !== 'http:' && parsedURL.protocol !== 'https:') {
      return null
    }
  } catch {
    return null
  }

  return (
    <a
      href={website}
      target='_blank'
      rel='noopener noreferrer'
      aria-label={t('Open {{name}} official website', {
        name: props.channelName,
      })}
      title={website}
      className='text-primary inline-flex max-w-full items-center gap-1 underline-offset-3 hover:underline'
      onClick={(event) => event.stopPropagation()}
    >
      <span className='truncate'>{website}</span>
      <ExternalLink className='size-3 shrink-0' aria-hidden='true' />
    </a>
  )
}
