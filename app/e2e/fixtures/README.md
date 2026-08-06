# Fixtures de e2e

## `colors-64x64.heic` (499 bytes)

HEIC **real, codificado em HEVC** (`ftyp heic`), 64×64 px — uma imagem sintética
de blocos de cor (sem pessoas, sem autoria criativa; efetivamente domínio
público). Serve a um único teste: `e2e/heic.spec.ts` — provar que o decodificador
HEIC (`heic2any` + WASM) que o app empacota **realmente decodifica** um HEIC num
navegador de verdade. Os testes unitários (`src/test/heic.test.ts`) cobrem a
fiação com o `heic2any` mockado; o decode WASM em si só se prova em browser, e é
o que este arquivo permite rodar no CI.

**Procedência:** arquivo de teste do projeto **libheif**
(`strukturag/libheif`, `fuzzing/data/corpus/colors-no-alpha.heic`). O libheif
distribui a biblioteca sob LGPL e os utilitários/wrappers sob MIT; este é um
arquivo de teste gerado (blocos de cor), usado aqui apenas como entrada de
decodificação. Mantido pequeno de propósito — decodifica em ~180 ms.

**Por que não uma foto real de iPhone:** um HEIC de iPhone tem ~2 MB / 12 MP e
levaria ~3 s por teste, além de carregar peso e direitos de imagem para dentro
do repositório. Para provar que o decode funciona, uma imagem sintética de 499 B
basta e é mais honesta como fixture.
