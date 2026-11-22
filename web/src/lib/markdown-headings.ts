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
export interface MarkdownHeading {
  id: string
  level: 2 | 3
  text: string
}

export function createMarkdownHeadingSlugger() {
  const counts = new Map<string, number>()
  const used = new Set<string>()

  return (rawText: string): string => {
    const cleaned = rawText
      .trim()
      .toLowerCase()
      .replaceAll(/[`*_~[\]()]/g, '')
      .replaceAll(/[^\p{L}\p{N}\s-]/gu, '')
      .replaceAll(/\s+/g, '-')
      .replaceAll(/-+/g, '-')
      .replaceAll(/^-|-$/g, '')
    const base = cleaned || 'section'
    let next = (counts.get(base) ?? 0) + 1
    let id = next === 1 ? base : `${base}-${next}`
    while (used.has(id)) {
      next++
      id = `${base}-${next}`
    }
    counts.set(base, next)
    used.add(id)
    return id
  }
}
