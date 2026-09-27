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

// Scenery spans the hero. CSS motion is gated by the hero's visibility and
// reduced-motion state; no extra RAF loops, textures or network requests.
export function HeroCosmicBackdrop() {
  return (
    <div className='benefit-cinema-cosmos' aria-hidden='true'>
      <div className='benefit-cinema-deep-stars' />
      <div className='benefit-cinema-deep-stars benefit-cinema-deep-stars-offset' />
      <div className='benefit-cinema-nebula' />
      {['distant', 'near'].map((depth) => (
        <div
          key={depth}
          className={`benefit-cinema-galaxy benefit-cinema-galaxy-${depth}`}
        >
          <svg viewBox='0 0 300 160' fill='none' focusable='false'>
            <g
              className='benefit-cinema-galaxy-arms'
              stroke='currentColor'
              strokeLinecap='round'
            >
              <path
                d='M151 80 C184 48 247 52 251 79 C256 118 127 146 60 115 C-1 87 51 28 144 27 C223 26 283 52 286 77'
                strokeWidth='1.2'
                opacity='.5'
              />
              <path
                d='M147 79 C123 109 64 105 62 84 C57 51 168 29 231 52 C282 73 237 121 165 128 C94 136 22 113 15 86'
                strokeWidth='1.4'
                opacity='.65'
              />
              <path
                d='M101 92 C117 107 189 95 199 76 C210 51 130 57 103 79 C72 106 175 126 227 94'
                strokeWidth='2'
                opacity='.35'
              />
            </g>
            <g className='benefit-cinema-galaxy-dust' fill='currentColor'>
              <circle cx='69' cy='51' r='1.4' />
              <circle cx='232' cy='107' r='1.6' />
              <circle cx='192' cy='33' r='1' />
              <circle cx='35' cy='98' r='1' />
              <circle cx='265' cy='64' r='1.2' />
            </g>
          </svg>
        </div>
      ))}
      <div className='benefit-cinema-foreground-planet' />
    </div>
  )
}
