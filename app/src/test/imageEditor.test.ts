import { describe, it, expect } from 'vitest'
import { outputSize } from '@/lib/imageEditor'

// Geometria pura do editor (o canvas real é coberto no e2e imageEditor.spec.ts).
describe('outputSize', () => {
  it('sem rotação nem recorte devolve o tamanho da fonte', () => {
    expect(outputSize(800, 600, 0)).toEqual({ w: 800, h: 600 })
  })

  it('rotação de 90/270 troca largura por altura', () => {
    expect(outputSize(800, 600, 90)).toEqual({ w: 600, h: 800 })
    expect(outputSize(800, 600, 270)).toEqual({ w: 600, h: 800 })
    expect(outputSize(800, 600, 180)).toEqual({ w: 800, h: 600 })
  })

  it('recorta primeiro (espaço original), depois rotaciona', () => {
    // metade em cada eixo → 1/4 da área.
    expect(outputSize(800, 600, 0, { x: 0.25, y: 0.25, w: 0.5, h: 0.5 })).toEqual({
      w: 400,
      h: 300,
    })
    // recorte 50% → 400x300; rotação 90 troca → 300x400.
    expect(outputSize(800, 600, 90, { x: 0, y: 0, w: 0.5, h: 0.5 })).toEqual({ w: 300, h: 400 })
  })

  it('nunca devolve dimensão zero', () => {
    expect(outputSize(800, 600, 0, { x: 0, y: 0, w: 0, h: 0 })).toEqual({ w: 1, h: 1 })
  })

  it('normaliza rotações fora do padrão para múltiplos de 90', () => {
    expect(outputSize(800, 600, 89)).toEqual({ w: 600, h: 800 }) // ~90
    expect(outputSize(800, 600, -90)).toEqual({ w: 600, h: 800 }) // 270
    expect(outputSize(800, 600, 360)).toEqual({ w: 800, h: 600 }) // 0
  })
})
