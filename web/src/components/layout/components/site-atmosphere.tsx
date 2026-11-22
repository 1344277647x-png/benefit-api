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
import { useEffect, useRef } from 'react'

// A single CSS scene shared by all routes; it never captures pointer events.
export function SiteAtmosphere() {
  const scene = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const element = scene.current
    if (!element) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')
    const updateMotion = () => {
      element.dataset.motion =
        document.visibilityState === 'visible' && !reduced.matches
          ? 'running'
          : 'paused'
    }
    updateMotion()
    document.addEventListener('visibilitychange', updateMotion)
    reduced.addEventListener('change', updateMotion)
    return () => {
      document.removeEventListener('visibilitychange', updateMotion)
      reduced.removeEventListener('change', updateMotion)
    }
  }, [])

  return (
    <div ref={scene} className='benefit-site-atmosphere' aria-hidden='true'>
      <div className='benefit-site-nebula' />
      <svg
        className='benefit-site-stars'
        viewBox='0 0 1440 900'
        fill='currentColor'
        focusable='false'
      >
        {[
          [81, 145],
          [227, 65],
          [450, 192],
          [618, 96],
          [916, 60],
          [1252, 124],
          [1378, 338],
          [1175, 491],
          [1068, 719],
          [1400, 823],
          [751, 815],
          [325, 732],
          [74, 524],
          [590, 553],
          [826, 349],
        ].map(([x, y], index) => (
          <circle key={x} cx={x} cy={y} r={index % 3 === 0 ? 1.5 : 0.8} />
        ))}
      </svg>
      <div className='benefit-site-orbit benefit-site-orbit-far'>
        <svg viewBox='0 0 400 400' fill='none' focusable='false'>
          <circle
            cx='200'
            cy='200'
            r='167'
            stroke='currentColor'
            strokeDasharray='330 115 90 514'
          />
          <circle
            cx='200'
            cy='200'
            r='137'
            stroke='currentColor'
            opacity='.45'
          />
          <circle cx='200' cy='33' r='4' fill='currentColor' />
        </svg>
      </div>
      <div className='benefit-site-orbit benefit-site-orbit-near'>
        <svg viewBox='0 0 400 400' fill='none' focusable='false'>
          <ellipse cx='200' cy='200' rx='190' ry='115' stroke='currentColor' />
          <ellipse
            cx='200'
            cy='200'
            rx='176'
            ry='98'
            stroke='currentColor'
            opacity='.4'
          />
          <circle cx='10' cy='200' r='3' fill='currentColor' />
        </svg>
      </div>
      <div className='benefit-site-planet' />
    </div>
  )
}
