// Conversão de HEIC/HEIF → JPEG NO CLIENTE.
//
// PORQUÊ: o iPhone salva foto como HEIC por padrão, e esse é o caminho MAIS
// comum na loja ("fotografa cada produto no celular"). Nem o <canvas> do
// navegador (fora do Safari) nem o backend — que só aceita jpeg/png/webp —
// decodificam HEIC, então a foto do iPhone era silenciosamente RECUSADA: sumia
// do lote ou estourava no upload. Convertê-la aqui, antes de comprimir e subir,
// resolve sem tocar no backend nem exigir libheif no servidor (dependência de
// sistema que a gente não quer no deploy).
//
// O decodificador (heic2any, ~1,4 MB de WASM) é carregado SOB DEMANDA (import
// dinâmico): quem só sobe JPEG nunca paga esse peso no bundle.

const HEIC_EXT = /\.(heic|heif)$/i

/**
 * HEIC é reconhecido pelo tipo MIME OU pela extensão. A extensão é essencial
 * porque, ao ARRASTAR uma pasta, o navegador quase sempre entrega os .heic com
 * `type` vazio (ou `application/octet-stream`) — aí só o nome do arquivo
 * denuncia o formato. Quando o MIME já traz um tipo de imagem conhecido,
 * respeitamos o MIME (não forçamos HEIC por causa da extensão).
 */
export function isHeic(file: File): boolean {
  const t = file.type.toLowerCase()
  if (t === 'image/heic' || t === 'image/heif') return true
  if (HEIC_EXT.test(file.name) && (t === '' || t === 'application/octet-stream')) return true
  return false
}

/**
 * Converte um File HEIC/HEIF para um File JPEG. Import dinâmico do heic2any: o
 * WASM só entra em cena quando aparece um HEIC de verdade. Mantém o nome do
 * arquivo (troca a extensão por .jpg) para o casamento por SKU seguir valendo.
 */
export async function heicToJpeg(file: File): Promise<File> {
  // heic2any referencia `window`/`Worker` no topo do módulo — por isso o import
  // é dinâmico e SÓ aqui (nunca no topo, senão quebra teste/SSR).
  const mod = await import('heic2any')
  const convert = (mod.default ?? mod) as (opts: {
    blob: Blob
    toType?: string
    quality?: number
  }) => Promise<Blob | Blob[]>
  const out = await convert({ blob: file, toType: 'image/jpeg', quality: 0.9 })
  // A API pode devolver um Blob ou uma lista (imagem HEIC com múltiplos frames);
  // a capa/foto do produto é sempre o primeiro frame.
  const blob = Array.isArray(out) ? out[0] : out
  const name = file.name.replace(HEIC_EXT, '') + '.jpg'
  return new File([blob], name, { type: 'image/jpeg', lastModified: file.lastModified })
}
