import { describe, it, expect } from 'vitest'
import { resolveImageUrl } from '@/lib/api'

// Regressão: "sumiram as fotos da vitrine". O catalog-service serve as imagens
// re-ingeridas em GET /media/*, e a API devolve a URL como caminho
// ROOT-RELATIVE ("/media/produtos/.../medium.jpg"). O navegador resolvia isso
// contra a origem da SPA (:5175), não contra o catalog (:8091) — dava 404 e o
// card caía no emoji. resolveImageUrl prefixa a base do catalog nas relativas.
describe('resolveImageUrl', () => {
  const base = 'http://192.168.0.143:8091'

  it('prefixa a base do catalog em URL /media relativa (o bug)', () => {
    expect(resolveImageUrl('/media/produtos/abc/x-medium.jpg', base)).toBe(
      'http://192.168.0.143:8091/media/produtos/abc/x-medium.jpg'
    )
  })

  it('prefixa também quando vem sem a barra inicial', () => {
    expect(resolveImageUrl('media/produtos/abc/x.jpg', base)).toBe(
      'http://192.168.0.143:8091/media/produtos/abc/x.jpg'
    )
  })

  it('não gera barra dupla quando a base termina em /', () => {
    expect(resolveImageUrl('/media/x.jpg', 'http://host:8091/')).toBe(
      'http://host:8091/media/x.jpg'
    )
  })

  it('deixa URL absoluta (Wikimedia/picsum) intacta', () => {
    const wm = 'https://upload.wikimedia.org/wikipedia/commons/thumb/x/960px-y.jpg'
    expect(resolveImageUrl(wm, base)).toBe(wm)
    expect(resolveImageUrl('http://picsum.photos/seed/x/800/800', base)).toBe(
      'http://picsum.photos/seed/x/800/800'
    )
  })

  it('deixa URL protocol-relative e data:/blob: intactas', () => {
    expect(resolveImageUrl('//cdn.exemplo.com/a.jpg', base)).toBe('//cdn.exemplo.com/a.jpg')
    expect(resolveImageUrl('data:image/png;base64,AAAA', base)).toBe('data:image/png;base64,AAAA')
  })

  it('sem base (mock mode) devolve a URL como veio — não inventa origem', () => {
    expect(resolveImageUrl('/media/produtos/abc/x.jpg', '')).toBe('/media/produtos/abc/x.jpg')
  })

  it('url vazia/undefined passa sem quebrar', () => {
    expect(resolveImageUrl(undefined, base)).toBeUndefined()
    expect(resolveImageUrl('', base)).toBe('')
  })
})
