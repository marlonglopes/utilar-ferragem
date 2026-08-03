import { describe, it, expect, vi } from 'vitest'
import {
  resolveBySku,
  runPool,
  skuCandidates,
  skuFromFilename,
  uploadProductImage,
} from '@/lib/adminBulkImagesApi'
import { compressImage } from '@/lib/imageCompress'

describe('skuCandidates — casa a foto ao produto pelo nome (robusto)', () => {
  it('tira caminho e extensão', () => {
    expect(skuCandidates('6320.jpg')).toEqual(['6320'])
    expect(skuCandidates('/fotos/6320.JPG')).toEqual(['6320'])
    expect(skuCandidates('C:\\fotos\\6320.png')).toEqual(['6320'])
  })
  it('várias fotos do mesmo produto: tenta o cheio e depois sem o -N', () => {
    // nome cheio primeiro (não casa), depois o SKU sem sufixo (casa).
    expect(skuCandidates('6320-2.jpg')).toEqual(['6320-2', '6320'])
    expect(skuCandidates('6320_3.jpg')).toEqual(['6320_3', '6320'])
  })
  it('SKU que termina em -dígitos NÃO quebra (casa pelo nome cheio)', () => {
    // UTL-FER-0007 é candidato ANTES de UTL-FER, então casa pelo cheio.
    expect(skuCandidates('UTL-FER-0007.jpg')).toEqual(['UTL-FER-0007', 'UTL-FER'])
    expect(skuFromFilename('UTL-FER-0007.jpg')).toBe('UTL-FER') // fallback de exibição
  })
  it('espaço antes do número também é sufixo de "foto N"', () => {
    expect(skuCandidates('6320 2.jpg')).toEqual(['6320 2', '6320'])
  })
  it('apara espaços em volta do nome', () => {
    expect(skuCandidates('  6320 .jpg')).toEqual(['6320'])
  })
  it('sem extensão usa o nome inteiro', () => {
    expect(skuCandidates('6320')).toEqual(['6320'])
  })
  it('só o sufixo (sem SKU) não vira candidato vazio', () => {
    // "-2.jpg": tirar o -2 deixaria "", então mantém o base e vira "sem produto".
    expect(skuCandidates('-2.jpg')).toEqual(['-2'])
  })
  it('múltiplos pontos: só a última extensão sai', () => {
    expect(skuCandidates('6320.v2.png')).toEqual(['6320.v2'])
  })
})

describe('adminBulkImagesApi (mock)', () => {
  it('resolveBySku casa SKUs numéricos no mock', async () => {
    const r = await resolveBySku(['6320', '7492', 'abc'])
    expect(r.length).toBe(2) // 'abc' não casa
    expect(r[0]).toHaveProperty('id')
    expect(r[0]).toHaveProperty('name')
    expect(r[0]).toHaveProperty('hasImage')
  })
  it('resolveBySku com lista vazia → []', async () => {
    expect(await resolveBySku([])).toEqual([])
  })
  it('uploadProductImage no mock resolve e reporta 100%', async () => {
    let last = 0
    await uploadProductImage('mock-6320', new File(['x'], '6320.jpg'), (p) => (last = p))
    expect(last).toBe(100)
  })
})

describe('runPool — sobe em paralelo com teto de concorrência', () => {
  it('processa todos os itens', async () => {
    const done: number[] = []
    await runPool([1, 2, 3, 4, 5, 6, 7], async (n) => void done.push(n), 3)
    expect(done.sort((a, b) => a - b)).toEqual([1, 2, 3, 4, 5, 6, 7])
  })
  it('nunca passa do teto de concorrência (e realmente paraleliza)', async () => {
    let inFlight = 0
    let max = 0
    await runPool(
      Array.from({ length: 12 }),
      async () => {
        inFlight++
        max = Math.max(max, inFlight)
        await new Promise((r) => setTimeout(r, 5))
        inFlight--
      },
      4
    )
    expect(max).toBeLessThanOrEqual(4)
    expect(max).toBeGreaterThan(1)
  })
  it('concorrência 0 vira 1 (não trava a fila)', async () => {
    const done: number[] = []
    await runPool([1, 2], async (n) => void done.push(n), 0)
    expect(done).toEqual([1, 2])
  })
  it('lista vazia resolve sem chamar o worker', async () => {
    const worker = vi.fn()
    await runPool([], worker, 5)
    expect(worker).not.toHaveBeenCalled()
  })
})

describe('compressImage — sem canvas (node/happy-dom) devolve o arquivo', () => {
  it('não-imagem passa intacto', async () => {
    const f = new File(['x'], 'a.txt', { type: 'text/plain' })
    expect(await compressImage(f)).toBe(f)
  })
  it('imagem sem createImageBitmap disponível volta o original', async () => {
    const f = new File(['x'], '6320.jpg', { type: 'image/jpeg' })
    // happy-dom não implementa createImageBitmap → cai no fallback e devolve f.
    expect(await compressImage(f)).toBe(f)
  })
})
