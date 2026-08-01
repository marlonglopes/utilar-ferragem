import { describe, it, expect } from 'vitest'
import {
  resolveBySku,
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
