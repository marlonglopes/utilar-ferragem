#!/usr/bin/env node
// Gerador de carga para o UPLOAD EM LOTE de imagens por SKU.
//
// Cria uma árvore de pastas (1 pasta por SKU, N fotos por pasta) com imagens
// JPEG mínimas válidas — pra arrastar na UI (Admin → Imagens em lote) e medir,
// num NAVEGADOR REAL, o que jsdom não mede: memória, DOM, travamento e o 414 da
// consulta by-sku. Os arquivos são minúsculos (~0,3 KB) de propósito: o gargalo
// que queremos medir é a QUANTIDADE, não o tamanho de cada foto.
//
// Uso:
//   node scripts/loadtest/gen-images.mjs [--folders N] [--per K] [--out DIR] [--skus arquivo]
//
// Exemplos:
//   node scripts/loadtest/gen-images.mjs                 # 500 pastas x 3 fotos (fake)
//   node scripts/loadtest/gen-images.mjs --folders 500 --per 5
//   node scripts/loadtest/gen-images.mjs --skus skus.txt # 1 pasta por SKU real (um por linha)
//
// Padrão de saída: <scratchpad>/loadtest-images/<SKU>/<i>.jpg
// Depois: abra a UI, arraste a pasta loadtest-images inteira e observe.

import { mkdir, writeFile, rm, readFile } from 'node:fs/promises'
import { join } from 'node:path'

// JPEG 1x1 válido (branco), ~125 bytes — suficiente pra decodificar/comprimir.
const TINY_JPEG = Buffer.from(
  '/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRof' +
    'Hh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wgALCAABAAEBAREA/8QAFBAB' +
    'AAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA=',
  'base64'
)

function arg(name, def) {
  const i = process.argv.indexOf(`--${name}`)
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : def
}

const OUT = arg(
  'out',
  join(
    process.env.SCRATCHPAD ??
      '/tmp/marlon/claude-1000/-home-marlon-utilar-ferragem/1ee5c9fa-c650-4db2-8d05-5f911580f0c5/scratchpad',
    'loadtest-images'
  )
)
const PER = Number(arg('per', '3'))
const skusFile = arg('skus', '')

async function resolveSkus() {
  if (skusFile) {
    const raw = await readFile(skusFile, 'utf8')
    return raw
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean)
  }
  const n = Number(arg('folders', '500'))
  // SKUs sintéticos com cara de EAN-13 (o pior caso de tamanho de query).
  return Array.from({ length: n }, (_, i) => String(i).padStart(13, '0'))
}

async function main() {
  const skus = await resolveSkus()
  await rm(OUT, { recursive: true, force: true })
  await mkdir(OUT, { recursive: true })

  let files = 0
  for (const sku of skus) {
    const dir = join(OUT, sku)
    await mkdir(dir, { recursive: true })
    for (let i = 1; i <= PER; i++) {
      await writeFile(join(dir, `${i}.jpg`), TINY_JPEG)
      files++
    }
  }

  const totalKB = Math.round((files * TINY_JPEG.length) / 1024)
  console.log(`OK: ${skus.length} pastas x ${PER} = ${files} arquivos (~${totalKB} KB) em`)
  console.log(`    ${OUT}`)
  console.log('')
  console.log('Agora, no NAVEGADOR (não headless):')
  console.log('  1. Admin → Imagens em lote.')
  console.log('  2. Abra o DevTools → Performance/Memory e o Gerenciador de tarefas do Chrome.')
  console.log('  3. Arraste a pasta loadtest-images inteira para a área de upload.')
  console.log('  4. Observe: aparece algum item? a aba trava? a memória dispara?')
  console.log('     Network: a chamada /admin/products/by-sku volta 414 (URI Too Long)?')
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
