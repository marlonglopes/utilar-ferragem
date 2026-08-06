import { describe, it, expect, vi, afterEach } from 'vitest'
import { isHeic } from '@/lib/heic'

/**
 * HEIC = foto de iPhone. É o caminho MAIS comum na loja e era silenciosamente
 * recusado (canvas e backend não decodificam). Cobrimos a detecção (inclusive o
 * caso real do drag de pasta, em que o browser manda `type` vazio) e a
 * conversão via heic2any (mockado — o WASM real precisa de browser de verdade).
 */

describe('isHeic — reconhece foto de iPhone', () => {
  it('pelo MIME image/heic e image/heif', () => {
    expect(isHeic(new File([''], 'x', { type: 'image/heic' }))).toBe(true)
    expect(isHeic(new File([''], 'x', { type: 'image/heif' }))).toBe(true)
  })

  it('pela EXTENSÃO quando o browser manda type vazio (o caso do drag de pasta)', () => {
    expect(isHeic(new File([''], 'IMG_1234.HEIC', { type: '' }))).toBe(true)
    expect(isHeic(new File([''], 'foto.heif', { type: '' }))).toBe(true)
  })

  it('pela extensão com type application/octet-stream (alguns navegadores)', () => {
    expect(isHeic(new File([''], '6320.heic', { type: 'application/octet-stream' }))).toBe(true)
  })

  it('NÃO confunde jpeg/png/webp', () => {
    expect(isHeic(new File([''], '6320.jpg', { type: 'image/jpeg' }))).toBe(false)
    expect(isHeic(new File([''], 'x.png', { type: 'image/png' }))).toBe(false)
    expect(isHeic(new File([''], 'x.webp', { type: 'image/webp' }))).toBe(false)
  })

  it('respeita o MIME conhecido: extensão .heic mas type=image/jpeg → não é heic', () => {
    // Se o browser já classificou como jpeg, confiamos no MIME (não forçamos a
    // conversão por causa do nome).
    expect(isHeic(new File([''], 'weird.heic', { type: 'image/jpeg' }))).toBe(false)
  })
})

describe('heicToJpeg — converte via heic2any (import dinâmico, mockado)', () => {
  afterEach(() => {
    vi.doUnmock('heic2any')
    vi.resetModules()
  })

  it('devolve File image/jpeg e troca a extensão por .jpg (mantém o SKU no nome)', async () => {
    vi.resetModules()
    const convert = vi.fn().mockResolvedValue(new Blob(['jpegdata'], { type: 'image/jpeg' }))
    vi.doMock('heic2any', () => ({ default: convert }))
    const { heicToJpeg } = await import('@/lib/heic')

    const out = await heicToJpeg(new File(['heic'], 'IMG_1234.HEIC', { type: 'image/heic' }))
    expect(out.type).toBe('image/jpeg')
    expect(out.name).toBe('IMG_1234.jpg')
    expect(convert).toHaveBeenCalledOnce()
    // Pediu jpeg ao decodificador.
    expect(convert.mock.calls[0][0]).toMatchObject({ toType: 'image/jpeg' })
  })

  it('heic2any devolvendo uma LISTA de frames: pega o primeiro (a capa)', async () => {
    vi.resetModules()
    const convert = vi.fn().mockResolvedValue([new Blob(['a'], { type: 'image/jpeg' })])
    vi.doMock('heic2any', () => ({ default: convert }))
    const { heicToJpeg } = await import('@/lib/heic')

    const out = await heicToJpeg(new File(['x'], '6320.heic'))
    expect(out.name).toBe('6320.jpg')
    expect(out.type).toBe('image/jpeg')
  })
})

describe('compressImage — converte HEIC antes (mesmo sem canvas)', () => {
  afterEach(() => {
    vi.doUnmock('heic2any')
    vi.resetModules()
  })

  it('HEIC entra, JPEG sai — sem canvas (ambiente de teste) devolve o convertido', async () => {
    vi.resetModules()
    const convert = vi.fn().mockResolvedValue(new Blob(['j'], { type: 'image/jpeg' }))
    vi.doMock('heic2any', () => ({ default: convert }))
    const { compressImage } = await import('@/lib/imageCompress')

    const out = await compressImage(new File(['h'], 'IMG.HEIC', { type: 'image/heic' }))
    expect(out.type).toBe('image/jpeg')
    expect(convert).toHaveBeenCalledOnce()
  })

  it('se a conversão HEIC falhar, devolve o arquivo cru (backend recusa com msg clara)', async () => {
    vi.resetModules()
    const convert = vi.fn().mockRejectedValue(new Error('wasm indisponível'))
    vi.doMock('heic2any', () => ({ default: convert }))
    const { compressImage } = await import('@/lib/imageCompress')

    const original = new File(['h'], 'IMG.HEIC', { type: 'image/heic' })
    const out = await compressImage(original)
    // Não trava: devolve o mesmo arquivo (não convertido).
    expect(out).toBe(original)
  })
})
