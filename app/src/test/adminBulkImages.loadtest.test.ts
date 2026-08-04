import { describe, it, expect } from 'vitest'
import { skuCandidates, runPool } from '@/lib/adminBulkImagesApi'

/**
 * TESTE DE CARGA (medição) do upload em lote — quantifica os limites de HOJE.
 *
 * Cenário motivador: o usuário fotografa tudo, organiza em 500 pastas (1 SKU por
 * pasta) e arrasta as 500 pastas na UI. Antes de otimizar, MEDIMOS onde dói.
 * Estes testes não sobem nada: eles caracterizam o comportamento atual com
 * números reprodutíveis (e falham se um limite regredir).
 */

const bytes = (s: string) => new TextEncoder().encode(s).length

/** Reproduz EXATAMENTE a URL que resolveBySku monta hoje (query única). */
function bySkuQuery(skus: string[]): string {
  return `/api/v1/admin/products/by-sku?skus=${encodeURIComponent(skus.join(','))}`
}

/** SKUs sintéticos com tamanho típico. EAN-13 (código de barras) = 13 dígitos. */
function fakeSkus(n: number, len = 13): string[] {
  return Array.from({ length: n }, (_, i) => String(i).padStart(len, '0'))
}

describe('CARGA · consulta by-sku é uma query só → risco de 414', () => {
  // Limites práticos comuns: nginx default ~8KB (large_client_header_buffers),
  // muitos proxies/gateways cortam em ~4-8KB. Acima disso: 414 URI Too Long.
  const LIMIT_8KB = 8 * 1024
  const counts = [100, 250, 500, 1000, 2000]

  it('tabela: bytes da URL por nº de SKUs (EAN-13)', () => {
    const rows = counts.map((n) => {
      // Como no fluxo real: cada arquivo gera candidatos (base + variante -N).
      // Numérico puro não gera variante, então ~1 candidato por SKU aqui; a
      // coluna "com variantes" simula nomes tipo 6320-2 (2 candidatos).
      const skus = fakeSkus(n)
      const q1 = bytes(bySkuQuery(skus))
      const withVariants = skus.flatMap((s) => [s, `${s}-2`])
      const q2 = bytes(bySkuQuery(withVariants))
      return { n, urlBytes: q1, urlBytesComVariantes: q2, passaDe8KB: q2 > LIMIT_8KB }
    })
    // Saída visível no runner — é o "relatório" do teste de carga.
    // eslint-disable-next-line no-console
    console.table(rows)

    // 500 SKUs já passa de 4KB — zona de risco em muitos proxies.
    const at500 = bytes(bySkuQuery(fakeSkus(500)))
    expect(at500).toBeGreaterThan(4 * 1024)

    // Com variantes, ~500 SKUs já estoura 8KB (414 provável).
    const at500v = bytes(bySkuQuery(fakeSkus(500).flatMap((s) => [s, `${s}-2`])))
    expect(at500v).toBeGreaterThan(LIMIT_8KB)
  })

  it('acha o nº de SKUs onde a URL cruza 8KB (documenta o teto de hoje)', () => {
    let n = 0
    while (bytes(bySkuQuery(fakeSkus(n))) <= LIMIT_8KB && n < 5000) n += 10
    // eslint-disable-next-line no-console
    console.log(`by-sku (query única) cruza 8KB em ~${n} SKUs EAN-13`)
    expect(n).toBeGreaterThan(0)
    expect(n).toBeLessThan(2000) // ou seja: 500-1000 SKUs já é arriscado
  })
})

describe('CARGA · runPool aguenta o volume, mas processa 5 por vez', () => {
  it('1500 itens: processa todos, nunca passa da concorrência', async () => {
    const N = 1500
    let done = 0
    let inFlight = 0
    let maxInFlight = 0
    await runPool(
      Array.from({ length: N }),
      async () => {
        inFlight++
        maxInFlight = Math.max(maxInFlight, inFlight)
        // microtask só pra permitir interleaving real do pool
        await Promise.resolve()
        done++
        inFlight--
      },
      5
    )
    expect(done).toBe(N)
    expect(maxInFlight).toBeLessThanOrEqual(5)
    // eslint-disable-next-line no-console
    console.log(`runPool: ${N} itens, no máximo ${maxInFlight} em voo (pool=5)`)
  })
})

describe('CARGA · casamento por PASTA é impossível hoje', () => {
  it('o SKU está na pasta, mas o casamento usa só o nome do arquivo', () => {
    // Um arquivo dentro da pasta do SKU chega como File cujo `.name` é só o
    // nome do arquivo (ex.: "IMG_0001.jpg") — o nome da pasta (o SKU) NÃO entra.
    const nomeDoArquivoNaPasta = 'IMG_0001.jpg' // pasta seria "6320/"
    const candidatos = skuCandidates(nomeDoArquivoNaPasta)
    // Documenta o gap: os candidatos vêm do NOME DO ARQUIVO (e ainda por cima o
    // "-0001" é interpretado como "foto N", virando lixo tipo "IMG"), e o SKU da
    // pasta ("6320") NUNCA aparece. Casar por pasta é impossível hoje.
    expect(candidatos).not.toContain('6320')
    expect(candidatos).toEqual(['IMG_0001', 'IMG'])
  })
})
