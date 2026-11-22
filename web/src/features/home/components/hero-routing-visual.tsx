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
import { Asterisk, Cpu, Sparkles } from 'lucide-react'
import type { PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Hero3DScene } from './hero-3d-scene'

// Decorative protocol map, not live channel health or availability data.
const nodes = [
  { name: 'OpenAI', protocol: '/v1/responses', icon: Cpu, position: 'openai' },
  {
    name: 'Claude',
    protocol: '/v1/messages',
    icon: Asterisk,
    position: 'claude',
  },
  { name: 'Gemini', protocol: '/v1beta', icon: Sparkles, position: 'gemini' },
] as const

const stars = [
  [48, 220],
  [85, 35],
  [180, 56],
  [250, 100],
  [366, 40],
  [488, 78],
  [551, 143],
  [568, 320],
  [518, 409],
  [431, 468],
  [316, 490],
  [224, 456],
  [69, 459],
  [30, 355],
  [152, 293],
  [449, 154],
  [380, 370],
  [505, 288],
  [113, 171],
  [294, 48],
  [567, 465],
  [26, 105],
  [214, 362],
  [452, 332],
] as const

function updateModelTilt(event: PointerEvent<HTMLDivElement>) {
  const card = event.currentTarget
  if (
    event.pointerType !== 'mouse' ||
    !window.matchMedia('(hover: hover) and (pointer: fine)').matches ||
    card.closest<HTMLElement>('.benefit-cinema-hero')?.dataset
      .decorativeMotion !== 'running'
  ) {
    return
  }
  // Measure the fixed wrapper, not the tilted surface, to avoid hover jitter.
  const bounds = card.getBoundingClientRect()
  if (!bounds.width || !bounds.height) return
  const x = Math.max(
    0,
    Math.min(1, (event.clientX - bounds.left) / bounds.width)
  )
  const y = Math.max(
    0,
    Math.min(1, (event.clientY - bounds.top) / bounds.height)
  )
  card.style.setProperty('--node-tilt-x', `${(0.5 - y) * 14}deg`)
  card.style.setProperty('--node-tilt-y', `${(x - 0.5) * 18}deg`)
  card.style.setProperty('--node-light-x', `${x * 100}%`)
  card.style.setProperty('--node-light-y', `${y * 100}%`)
  card.dataset.hovered = 'true'
}

function resetModelTilt(event: PointerEvent<HTMLDivElement>) {
  delete event.currentTarget.dataset.hovered
}

export function HeroRoutingVisual() {
  const { t } = useTranslation()
  return (
    <div className='benefit-cinema-visual' aria-hidden='true'>
      <svg
        className='benefit-cinema-star-map'
        viewBox='0 0 600 530'
        fill='none'
        focusable='false'
        preserveAspectRatio='none'
      >
        <g className='benefit-cinema-stars' fill='currentColor'>
          {stars.map(([x, y], index) => (
            <circle
              key={x}
              cx={x}
              cy={y}
              r={index % 4 === 0 ? 1.8 : 0.9}
              opacity={index % 3 === 0 ? 0.7 : 0.3}
            />
          ))}
        </g>
        <g
          className='benefit-cinema-orbits'
          stroke='currentColor'
          strokeWidth='0.8'
        >
          <ellipse
            cx='300'
            cy='265'
            rx='254'
            ry='112'
            transform='rotate(-32 300 265)'
          />
          <ellipse
            cx='300'
            cy='265'
            rx='228'
            ry='146'
            transform='rotate(36 300 265)'
            strokeDasharray='3 9'
            opacity='0.55'
          />
          <path
            d='M72 160 C168 136 164 270 300 265 S430 180 530 256 M126 422 C140 324 218 358 300 265'
            opacity='0.7'
          />
        </g>
        <g className='benefit-cinema-route-points' fill='currentColor'>
          <circle cx='188' cy='132' r='3' />
          <circle cx='479' cy='357' r='3.5' />
          <circle cx='232' cy='396' r='2.5' />
        </g>
        <g
          className='benefit-cinema-reticle'
          stroke='currentColor'
          strokeWidth='1'
        >
          <path d='M282 56 H318 M300 38 V74 M543 365 H561 M552 356 V374 M67 316 H85 M76 307 V325' />
          <path d='M22 78 V56 H44 M556 474 H578 V452' />
        </g>
      </svg>
      <div className='benefit-cinema-light-trail' />
      <div className='benefit-cinema-halo' />
      <Hero3DScene />
      {nodes.map((node) => {
        const Icon = node.icon
        return (
          <div
            key={node.name}
            className={`benefit-cinema-node benefit-cinema-node-${node.position}`}
            onPointerMove={updateModelTilt}
            onPointerLeave={resetModelTilt}
            onPointerCancel={resetModelTilt}
          >
            <div className='benefit-cinema-node-surface'>
              <Icon className='benefit-cinema-node-icon' strokeWidth={1.3} />
              <div className='benefit-cinema-node-copy'>
                <span className='benefit-cinema-node-name'>{node.name}</span>
                <span className='benefit-cinema-node-protocol'>
                  {node.protocol}
                </span>
              </div>
            </div>
          </div>
        )
      })}
      <span className='benefit-cinema-orbit-label'>
        {t('OpenAI-compatible access')}
      </span>
    </div>
  )
}
