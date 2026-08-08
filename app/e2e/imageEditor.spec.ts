import { test, expect } from '@playwright/test'

/**
 * Canvas REAL do editor de imagem — o que os unitários não fazem (happy-dom não
 * tem canvas/toBlob). Roda num chromium de verdade: importa o módulo-fonte do
 * app via Vite, rotaciona+recorta uma imagem sintética e confirma que a saída é
 * um JPEG decodificável cujas dimensões batem com a geometria pura (outputSize).
 */
test.describe('imageEditor — canvas real', () => {
  test('renderEditedBlob rotaciona + recorta e produz JPEG das dimensões corretas', async ({
    page,
  }) => {
    await page.goto('/')

    const result = await page.evaluate(async () => {
      const mod = await import('/src/lib/imageEditor.ts')

      // Imagem sintética 100x60 (duas faixas de cor).
      const src = document.createElement('canvas')
      src.width = 100
      src.height = 60
      const sctx = src.getContext('2d')!
      sctx.fillStyle = '#f47920'
      sctx.fillRect(0, 0, 50, 60)
      sctx.fillStyle = '#1b3e8a'
      sctx.fillRect(50, 0, 50, 60)
      const bmp = await createImageBitmap(src)

      // Recorta a metade esquerda (50x60), depois gira 90° → 60x50.
      const edit = { rotation: 90, crop: { x: 0, y: 0, w: 0.5, h: 1 } }
      const blob = await mod.renderEditedBlob(bmp, edit, 'image/jpeg', 0.9)

      const head = new Uint8Array(await blob.slice(0, 3).arrayBuffer())
      const magic = head[0] === 0xff && head[1] === 0xd8 && head[2] === 0xff
      const out = await createImageBitmap(blob)
      const expected = mod.outputSize(100, 60, edit.rotation, edit.crop)
      return { magic, w: out.width, h: out.height, expected, type: blob.type }
    })

    expect(result.magic, 'saída deve ser JPEG (FFD8)').toBe(true)
    expect(result.type).toBe('image/jpeg')
    expect({ w: result.w, h: result.h }).toEqual(result.expected)
    expect(result.expected).toEqual({ w: 60, h: 50 })
  })
})
