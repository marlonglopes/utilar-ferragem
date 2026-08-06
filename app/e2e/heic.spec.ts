import { test, expect } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

/**
 * DECODE HEIC REAL — o teste que os unitários NÃO conseguem fazer.
 *
 * `src/test/heic.test.ts` roda em happy-dom com o `heic2any` MOCKADO: prova a
 * fiação (detecção, troca de extensão, fallback), mas NÃO o decode WASM em si —
 * happy-dom não tem Worker + WebAssembly. Aqui rodamos o decodificador REAL (a
 * mesmíssima versão de `heic2any` que o app empacota) num chromium de verdade,
 * sobre um HEIC real codificado em HEVC (a fixture de 499 B), e conferimos que a
 * saída é um JPEG que o navegador DECODIFICA (dimensões reais > 0) — não lixo.
 *
 * Injetamos o dist do heic2any direto (em vez de importar o módulo do app via
 * Vite) de propósito: evita o flake do otimizador de dependências do Vite, que
 * recarrega a página no primeiro import dinâmico. É a mesma dependência e o
 * mesmo caminho de decode; a fiação do wrapper `heicToJpeg` já é coberta no unit.
 */

const here = dirname(fileURLToPath(import.meta.url))
const heicB64 = readFileSync(join(here, 'fixtures/colors-64x64.heic')).toString('base64')
const heic2anyDist = readFileSync(
  join(here, '../node_modules/heic2any/dist/heic2any.min.js'),
  'utf8'
)

test.describe('HEIC — decode real (foto de iPhone) no navegador', () => {
  test('converte um HEIC HEVC real em JPEG decodificável', async ({ page }) => {
    // about:blank: independe do bundle do app subir — o teste é do decode.
    await page.goto('about:blank')
    await page.addScriptTag({ content: heic2anyDist })

    const result = await page.evaluate(async (b64) => {
      const heic2any = (window as unknown as { heic2any?: unknown }).heic2any
      if (typeof heic2any !== 'function') {
        return { ok: false, reason: 'window.heic2any não foi exposto pelo dist' }
      }

      // base64 -> Blob HEIC, exatamente como um File de upload chegaria.
      const bin = atob(b64)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      const heicBlob = new Blob([bytes], { type: 'image/heic' })

      let out: Blob | Blob[]
      try {
        out = await (heic2any as (o: unknown) => Promise<Blob | Blob[]>)({
          blob: heicBlob,
          toType: 'image/jpeg',
          quality: 0.9,
        })
      } catch (e) {
        return { ok: false, reason: 'heic2any lançou: ' + (e instanceof Error ? e.message : e) }
      }
      const jpeg = Array.isArray(out) ? out[0] : out

      // 1) magic de JPEG: FF D8 FF
      const head = new Uint8Array(await jpeg.slice(0, 3).arrayBuffer())
      const magic = head[0] === 0xff && head[1] === 0xd8 && head[2] === 0xff

      // 2) o navegador DECODIFICA o JPEG de saída (prova que não é lixo).
      let w = 0
      let h = 0
      try {
        const bmp = await createImageBitmap(jpeg)
        w = bmp.width
        h = bmp.height
        bmp.close?.()
      } catch (e) {
        return {
          ok: false,
          reason: 'createImageBitmap falhou: ' + (e instanceof Error ? e.message : e),
        }
      }

      return { ok: magic && w > 0 && h > 0, type: jpeg.type, magic, w, h }
    }, heicB64)

    expect(result.ok, `decode falhou: ${JSON.stringify(result)}`).toBe(true)
    expect(result.type).toBe('image/jpeg')
    expect(result.magic).toBe(true)
    expect(result.w).toBe(64)
    expect(result.h).toBe(64)
  })
})
