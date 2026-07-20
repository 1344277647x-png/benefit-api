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
import * as THREE from 'three'

import { useTheme } from '@/context/theme-provider'

type AnimatedNode = {
  mesh: THREE.Mesh
  phase: number
}

export function Hero3DScene() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const { resolvedTheme } = useTheme()

  useEffect(() => {
    const canvas = canvasRef.current
    const host = canvas?.parentElement
    if (!canvas || !host) return

    const reducedMotion = window.matchMedia(
      '(prefers-reduced-motion: reduce)'
    ).matches
    const dark = resolvedTheme === 'dark'
    const palette = dark
      ? {
          primary: 0x35e4c4,
          secondary: 0xf4bd55,
          line: 0xa9fff0,
          core: 0x0b2924,
        }
      : {
          primary: 0x008f7b,
          secondary: 0xa66d00,
          line: 0x0a7165,
          core: 0xdaf8f1,
        }

    let renderer: THREE.WebGLRenderer
    try {
      renderer = new THREE.WebGLRenderer({
        canvas,
        alpha: true,
        antialias: true,
        powerPreference: 'high-performance',
      })
    } catch {
      canvas.dataset.webglState = 'unavailable'
      canvas.dataset.renderState = 'fallback'
      return
    }

    canvas.dataset.webglState = 'ready'
    renderer.setClearColor(0x000000, 0)
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.75))
    renderer.outputColorSpace = THREE.SRGBColorSpace
    renderer.toneMapping = THREE.ACESFilmicToneMapping
    renderer.toneMappingExposure = dark ? 1.2 : 0.95

    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(40, 1, 0.1, 80)
    camera.position.set(0, 0.45, 9.5)
    camera.lookAt(0, 0, 0)

    const network = new THREE.Group()
    scene.add(network)

    const coreGeometry = new THREE.TorusKnotGeometry(1.05, 0.2, 128, 18, 2, 3)
    const coreMaterial = new THREE.MeshPhysicalMaterial({
      color: palette.core,
      emissive: palette.primary,
      emissiveIntensity: dark ? 0.11 : 0.035,
      transparent: true,
      opacity: dark ? 0.46 : 0.34,
      transmission: dark ? 0.62 : 0.48,
      thickness: 0.85,
      roughness: 0.16,
      metalness: 0.08,
      clearcoat: 1,
      clearcoatRoughness: 0.16,
    })
    const core = new THREE.Mesh(coreGeometry, coreMaterial)
    core.rotation.set(0.35, -0.25, 0.2)
    network.add(core)

    const coreWire = new THREE.LineSegments(
      new THREE.WireframeGeometry(coreGeometry),
      new THREE.LineBasicMaterial({
        color: palette.line,
        transparent: true,
        opacity: dark ? 0.22 : 0.16,
      })
    )
    coreWire.rotation.copy(core.rotation)
    network.add(coreWire)

    const inner = new THREE.Mesh(
      new THREE.IcosahedronGeometry(0.67, 1),
      new THREE.MeshPhysicalMaterial({
        color: palette.primary,
        emissive: palette.primary,
        emissiveIntensity: dark ? 0.16 : 0.06,
        transparent: true,
        opacity: 0.2,
        wireframe: true,
        roughness: 0.28,
        metalness: 0.12,
      })
    )
    network.add(inner)

    const ringMaterial = new THREE.MeshBasicMaterial({
      color: palette.line,
      transparent: true,
      opacity: dark ? 0.26 : 0.18,
    })
    const rings = [
      new THREE.Mesh(
        new THREE.TorusGeometry(1.88, 0.018, 6, 128),
        ringMaterial
      ),
      new THREE.Mesh(
        new THREE.TorusGeometry(2.55, 0.014, 6, 128),
        ringMaterial
      ),
      new THREE.Mesh(
        new THREE.TorusGeometry(3.18, 0.012, 6, 128),
        ringMaterial
      ),
    ]
    rings[0].rotation.set(0.72, 0.08, 0.18)
    rings[1].rotation.set(1.12, 0.42, -0.28)
    rings[2].rotation.set(0.36, 1.04, 0.42)
    rings.forEach((ring) => network.add(ring))

    const animatedNodes: AnimatedNode[] = []
    const nodePoints: THREE.Vector3[] = []
    const nodeGeometry = new THREE.OctahedronGeometry(0.16, 0)
    for (let index = 0; index < 9; index += 1) {
      const angle = (index / 9) * Math.PI * 2
      const radius = index % 2 === 0 ? 2.55 : 3.15
      const point = new THREE.Vector3(
        Math.cos(angle) * radius,
        Math.sin(angle) * radius * 0.62,
        Math.sin(angle * 1.7) * 0.58
      )
      nodePoints.push(point)

      const node = new THREE.Mesh(
        nodeGeometry,
        new THREE.MeshPhysicalMaterial({
          color: index % 3 === 0 ? palette.secondary : palette.primary,
          emissive: index % 3 === 0 ? palette.secondary : palette.primary,
          emissiveIntensity: dark ? 0.24 : 0.08,
          transparent: true,
          opacity: dark ? 0.68 : 0.56,
          transmission: 0.32,
          roughness: 0.18,
          metalness: 0.16,
          clearcoat: 1,
        })
      )
      node.position.copy(point)
      network.add(node)
      animatedNodes.push({ mesh: node, phase: index * 0.7 })
    }

    const spokePositions: number[] = []
    nodePoints.forEach((point) => {
      spokePositions.push(0, 0, 0, point.x, point.y, point.z)
    })
    const spokesGeometry = new THREE.BufferGeometry()
    spokesGeometry.setAttribute(
      'position',
      new THREE.Float32BufferAttribute(spokePositions, 3)
    )
    const spokes = new THREE.LineSegments(
      spokesGeometry,
      new THREE.LineBasicMaterial({
        color: palette.line,
        transparent: true,
        opacity: dark ? 0.16 : 0.11,
      })
    )
    network.add(spokes)

    const circuitGeometry = new THREE.BufferGeometry().setFromPoints([
      ...nodePoints,
      nodePoints[0],
    ])
    const circuit = new THREE.Line(
      circuitGeometry,
      new THREE.LineBasicMaterial({
        color: palette.secondary,
        transparent: true,
        opacity: dark ? 0.18 : 0.13,
      })
    )
    network.add(circuit)

    const grid = new THREE.GridHelper(18, 28, palette.primary, palette.line)
    grid.position.set(0, -2.75, -0.8)
    const gridMaterials = Array.isArray(grid.material)
      ? grid.material
      : [grid.material]
    gridMaterials.forEach((material) => {
      material.transparent = true
      material.opacity = dark ? 0.07 : 0.045
    })
    scene.add(grid)

    scene.add(new THREE.AmbientLight(0xffffff, dark ? 0.75 : 1.15))
    const keyLight = new THREE.PointLight(palette.primary, dark ? 18 : 10, 18)
    keyLight.position.set(3.6, 4.2, 5.5)
    scene.add(keyLight)
    const accentLight = new THREE.PointLight(
      palette.secondary,
      dark ? 12 : 7,
      16
    )
    accentLight.position.set(-4.2, -1.8, 3.4)
    scene.add(accentLight)

    const pointer = { x: 0, y: 0 }
    const handlePointerMove = (event: PointerEvent) => {
      pointer.x = event.clientX / window.innerWidth - 0.5
      pointer.y = event.clientY / window.innerHeight - 0.5
    }

    let frameId = 0
    let lastWidth = 0
    let lastHeight = 0

    const renderFrame = (time: number) => {
      const elapsed = time * 0.001
      network.rotation.y = elapsed * 0.06 + pointer.x * 0.22
      network.rotation.x = Math.sin(elapsed * 0.35) * 0.025 - pointer.y * 0.12
      core.rotation.z = 0.2 + elapsed * 0.09
      coreWire.rotation.copy(core.rotation)
      inner.rotation.x = elapsed * 0.13
      inner.rotation.y = -elapsed * 0.17
      rings[0].rotation.z = 0.18 + elapsed * 0.045
      rings[1].rotation.y = 0.42 - elapsed * 0.038
      rings[2].rotation.x = 0.36 + elapsed * 0.032
      animatedNodes.forEach(({ mesh, phase }) => {
        const scale = 0.88 + Math.sin(elapsed * 1.5 + phase) * 0.12
        mesh.scale.setScalar(scale)
        mesh.rotation.x = elapsed * 0.24 + phase
        mesh.rotation.y = -elapsed * 0.2 + phase * 0.6
      })
      renderer.render(scene, camera)
      canvas.dataset.renderState = 'ready'
    }

    const resize = () => {
      const { width, height } = host.getBoundingClientRect()
      if (width <= 0 || height <= 0) return
      const nextWidth = Math.round(width)
      const nextHeight = Math.round(height)
      if (nextWidth === lastWidth && nextHeight === lastHeight) return
      lastWidth = nextWidth
      lastHeight = nextHeight
      renderer.setSize(nextWidth, nextHeight, false)
      camera.aspect = nextWidth / nextHeight
      camera.updateProjectionMatrix()

      const mobile = nextWidth < 640
      const compact = nextWidth < 960
      let networkScale = 1
      if (mobile) {
        networkScale = 0.72
      } else if (compact) {
        networkScale = 0.86
      }
      network.position.set(compact ? 0 : 2.45, mobile ? 0.25 : 0, 0)
      network.scale.setScalar(networkScale)
      grid.position.x = compact ? 0 : 1.7
      renderFrame(0)
    }

    const resizeObserver = new ResizeObserver(resize)
    resizeObserver.observe(host)
    resize()

    const animate = (time: number) => {
      renderFrame(time)
      frameId = window.requestAnimationFrame(animate)
    }

    if (reducedMotion) {
      renderFrame(0)
    } else {
      window.addEventListener('pointermove', handlePointerMove, {
        passive: true,
      })
      frameId = window.requestAnimationFrame(animate)
    }

    return () => {
      window.cancelAnimationFrame(frameId)
      window.removeEventListener('pointermove', handlePointerMove)
      resizeObserver.disconnect()
      scene.traverse((object) => {
        if (!('geometry' in object)) return
        const renderable = object as
          | THREE.Mesh
          | THREE.Line
          | THREE.LineSegments
        renderable.geometry?.dispose()
        const materials = Array.isArray(renderable.material)
          ? renderable.material
          : [renderable.material]
        materials.forEach((material) => material?.dispose())
      })
      renderer.dispose()
    }
  }, [resolvedTheme])

  return (
    <div className='benefit-hero-3d-stage' aria-hidden='true'>
      <canvas
        ref={canvasRef}
        className='benefit-hero-3d-canvas'
        data-scene='api-routing-core'
      />
    </div>
  )
}
