# Imagens de produto — upload, normalização e storage

Como a Utilar recebe, transforma, guarda e serve foto de produto.

> **Estado atual:** storage **local** (disco), funcionando. Produção usa **S3** —
> a troca é configuração (`STORAGE_DRIVER`), não mudança de código. Ver
> [S3 — o que falta](#s3--o-que-falta).

---

## Por que existe normalização

Foto de produto chega de todo jeito: retrato do celular, paisagem do fabricante,
PNG com fundo transparente, JPEG de 8 MB, imagem já espremida de 200px.

Sem normalizar, a vitrine vira um mosaico irregular — cada card com uma
proporção — e o celular baixa dezenas de megabytes para desenhar miniaturas de
150px. **Normalizar não é estética: é o que faz a loja abrir no 4G.**

---

## O pipeline, em ordem

### 1. Identificação pelos bytes

O tipo real é detectado pelo *magic number* do arquivo. **A extensão e o
`Content-Type` são ignorados** — os dois são controlados por quem envia.
`virus.exe` renomeado para `foto.jpg` é recusado.

Só JPEG, PNG, WebP e GIF passam (GIF só o primeiro quadro — catálogo não tem
imagem animada, e decodificar todos os quadros é o vetor de esgotamento de
memória do formato). O decoder é escolhido pelo formato detectado, e não
pelo registry genérico do Go: um `import _ "image/x"` futuro ampliaria em
silêncio o conjunto de formatos aceitos sem passar por essa checagem.

### 2. Rotação aplicada, metadados apagados

Foto de celular vem "deitada", com uma etiqueta EXIF dizendo como girar. A
rotação é aplicada **nos pixels**, e depois **todo o EXIF é descartado**.

> ⚠️ Isso não é limpeza cosmética. EXIF de celular carrega **coordenada GPS**.
> Publicar a foto crua publicaria a localização da loja — ou da casa de quem
> fotografou.

### 3. Enquadramento quadrado, sem deformar

Todas as imagens saem **1:1**. A escolha do quadrado é do padrão de e-commerce:
grade uniforme na vitrine, e funciona igual no card e no carrossel.

O encaixe usa **`contain`**, não `cover`:

| | o que faz | resultado |
|---|---|---|
| `contain` ✅ | usa a **menor** escala; o lado maior encosta na borda, o menor sobra em branco | produto inteiro, com margem |
| `cover` ❌ | usa a **maior** escala e corta o excedente | **corta a ponta da ferramenta** |
| esticar ❌ | força os dois lados | furadeira deformada |

O fundo é pintado de **branco antes** de desenhar. Sem isso, um PNG com
transparência sairia num quadro preto (RGBA zerado).

Efeito prático: a furadeira deitada e o saco de cimento em pé ocupam **o mesmo
espaço na grade**, e nenhum dos dois fica esticado ou cortado.

### 4. Três variantes, uma por uso

| Variante | Lado | Qualidade | Onde aparece |
|---|---:|---:|---|
| `thumb` | 300 px | 78 | card da vitrine, miniatura do carrossel |
| `medium` | 800 px | 82 | slide do carrossel no detalhe |
| `large` | 1600 px | 85 | zoom / tela cheia |

**Por que três e não uma:** a vitrine no celular mostra ~20 cards. Servir a
imagem de zoom neles é `20 × ~400 KB = 8 MB` para desenhar miniaturas de 150px —
é exatamente o que trava a navegação em 4G. Com o thumb, são `20 × ~25 KB`.

A qualidade do thumb é menor **de propósito**: em 300px o artefato de JPEG não é
perceptível, e cada KB ali é multiplicado pelo número de cards da página.

### 5. Nunca amplia

Se a original é menor que o lado da variante, ela **não é ampliada** — a variante
maior reaproveita a menor (alias). Ampliar não acrescenta informação: só peso e
borrão.

O `thumb` é a exceção e é sempre gerado, mesmo que exija ampliar um pouco — sem
ele a vitrine voltaria a baixar a imagem grande no card, que é o problema que
tudo isso resolve.

O frontend continua recebendo **as três chaves** em qualquer caso: a diferença
é invisível para ele, e o contrato não muda.

### 6. Recodificação sempre

Mesmo um JPEG já válido e já no tamanho certo é decodificado e reencodado.
**É isso que neutraliza payload escondido** dentro de uma imagem tecnicamente
válida — o arquivo que sai é gerado por nós, byte a byte.

---

## Segurança do upload

Upload é a superfície mais perigosa de qualquer aplicação. As defesas:

| Risco | Defesa |
|---|---|
| Arquivo que não é imagem | tipo detectado pelos bytes; extensão e `Content-Type` ignorados |
| Payload dentro de imagem válida | **recodificação obrigatória** |
| Bomba de descompressão (30.000×30.000 px) | teto de dimensão **antes** de decodificar |
| Corpo gigante | `MaxBytesReader` **antes** do parser multipart tocar o corpo |
| Travessia de caminho (`../../etc/passwd`) | o nome do cliente **nunca** vira caminho; a chave é gerada por nós (hash) |
| `/tmp` enchendo | temporários do parser removidos ao fim de cada requisição |
| Imagem patológica travando o servidor | timeout no processamento |
| Vazamento de GPS | EXIF removido |
| Upload por qualquer um | rota sob `admin` |

Limites: tamanho por arquivo, arquivos por requisição, e total de imagens por
produto — todos com mensagem que diz o limite e o valor recebido.

---

## Armazenamento

A camada é **plugável por interface**. O handler não sabe qual driver está ativo.

```
STORAGE_DRIVER=local   # desenvolvimento (padrão)
STORAGE_DRIVER=s3      # produção
```

### A regra que evita dor na migração

**O banco guarda a chave lógica, nunca a URL absoluta.**

```
✅ produtos/<id-do-produto>/<hash>-medium.jpg
❌ /var/lib/utilar/media/produtos/.../medium.jpg
❌ https://bucket.s3.amazonaws.com/produtos/.../medium.jpg
```

A URL pública é **derivada** da chave na hora de servir, a partir de um prefixo
configurável. Se a URL absoluta fosse gravada, migrar disco→S3 (ou trocar de CDN)
exigiria reescrever a tabela inteira — em produção, isso é parada.

### Deduplicação

Cada imagem carrega o **hash do conteúdo**, indexado. A mesma foto enviada duas
vezes não gera dois objetos, e reprocessar o acervo é idempotente.

### Convivência com imagem externa

O catálogo tem imagens que apontam para **URL de terceiro** (as fotos CC0 do
Wikimedia usadas como dado de teste). Elas não têm variantes e **não podem
quebrar**. O modelo comporta os dois casos:

| | `storage_key` | `variants` |
|---|---|---|
| imagem própria (upload) | preenchido | thumb/medium/large |
| imagem externa (URL) | nulo | nulo — usa `url` direto |

---

## API

Todas sob `/api/v1/admin`, exigindo papel `admin`.

| Método | Rota | O que faz |
|---|---|---|
| `GET` | `/products/by-id/:id/images` | lista as imagens do produto |
| `POST` | `/products/by-id/:id/images/upload` | **upload multipart** (campo `files`, repetível; `alt` opcional) |
| `PUT` | `/products/by-id/:id/images/order` | reordena |
| `PUT` | `/products/by-id/:id/images/:imageId/cover` | define a capa |
| `DELETE` | `/products/by-id/:id/images/:imageId` | remove |
| `POST` | `/products/by-id/:id/images` | adiciona por **URL** (caminho antigo, mantido) |

O upload devolve **201 se ao menos um arquivo entrou**, e o corpo traz o motivo
de cada recusa individualmente — um arquivo ruim não derruba o lote inteiro.
Se **todos** forem recusados, é **400** com `details` listando cada motivo.

### Contrato de resposta — o que o frontend consome

**`POST .../images/upload`** → `201`

```json
{
  "uploaded": [
    {
      "id": "af26b939-2270-4ba8-a6a1-53ac4c61de62",
      "alt": "furadeira de impacto",
      "sortOrder": 0,
      "url": "/media/produtos/<id>/1f3a…-large.jpg",
      "variants": {
        "thumb":  "/media/produtos/<id>/1f3a…-thumb.jpg",
        "medium": "/media/produtos/<id>/1f3a…-medium.jpg",
        "large":  "/media/produtos/<id>/1f3a…-large.jpg"
      },
      "width": 1600, "height": 1600,
      "originalBytes": 4821994, "bytes": 183220,
      "sourceFormat": "jpeg",
      "deduplicated": false
    }
  ],
  "rejected": [
    { "filename": "print.png", "reason": "…", "code": "not_an_image" }
  ]
}
```

`code` da recusa (casar por código, nunca pela mensagem):
`not_an_image` · `file_too_large` · `image_too_large` (bomba / dimensão) ·
`image_too_small` · `corrupt_image` · `processing_timeout` · `storage_error`.

`deduplicated: true` significa que a foto **já existia** no produto (mesmo hash
de conteúdo): o `id` devolvido é o da linha existente, e nenhuma nova entrou.

**`PUT .../images/order`** → corpo `{"order": ["<id>", "<id>", …]}` com a lista
**completa** na ordem desejada; o primeiro vira a capa. Idempotente.

**`PUT .../images/:imageId/cover`** → promove a capa sem mandar a lista inteira.

**`GET .../images`** → `{"data": [ …mesma forma de `uploaded`… ]}`.

### No payload público de produto

`ProductImage` ganhou campos, **de forma aditiva** — quem já lia `url` continua
funcionando:

```jsonc
{
  "id": "…",
  "url": "…",          // sempre preenchido: a melhor imagem para o contexto
  "alt": "…",
  "variants": {        // AUSENTE em imagem externa (legado Wikimedia)
    "thumb": "…", "medium": "…", "large": "…"
  },
  "width": 1600, "height": 1600
}
```

**Como distinguir os dois tipos:** `if (img.variants)` → imagem própria, escolha
o tamanho pelo contexto (`thumb` no card, `medium` no slide, `large` no zoom).
Sem `variants` → imagem externa por URL, use `url` como está.

`url` já vem resolvido para o contexto, então o caminho preguiçoso também é o
certo:

| Rota | `url` aponta para |
|---|---|
| `GET /products` (vitrine) | **`thumb`** — o card nunca baixa a imagem de zoom |
| `GET /products/:slug` (detalhe) | **`medium`** — o slide do carrossel |

Na listagem pública, o card recebe **só a capa** (`loadThumbnails`, uma query por
página, não N+1). A galeria completa é exclusiva do detalhe do produto.

### Limites em vigor

| Limite | Valor |
|---|---:|
| Tamanho por arquivo | 12 MB |
| Arquivos por requisição | 20 |
| Imagens por produto | 60 |
| Pixels da origem (bomba de descompressão) | 50 MP |
| Dimensão por lado | 12.000 px |
| Dimensão mínima (maior lado) | 200 px |
| Timeout por imagem | 20 s |
| Timeout do lote | 90 s |
| Corpo multipart | 64 MB |

---

## Ingestão em massa (dado de teste)

`scripts/ingestao/ingerir_imagens.py` busca fotos **CC0 / domínio público** no
Wikimedia Commons, baixa, e passa pelo **endpoint real de upload** — exercitando
o pipeline inteiro com imagem de verdade vinda da internet.

```bash
python3 scripts/ingestao/ingerir_imagens.py --dry-run
python3 scripts/ingestao/ingerir_imagens.py --por-produto 5
```

> Diferença para `imagens_commons.py`: aquele grava a URL do Wikimedia direto no
> banco (a foto continua hospedada lá). Este **baixa e ingere**, que é o caminho
> de produção.

### A busca escolhe o termo pelo TIPO do produto

O Commons é indexado em inglês, então há um mapa pt-BR → inglês. O detalhe que
importa: **palavra de embalagem não identifica o produto**.

"Arame Galvanizado nº 12 **Rolo** 1kg" é um arame vendido em rolo — casar "rolo"
trazia foto de **rolo de pintura**. Palavras como *rolo, caixa, saco, barra,
lata, cento, par* só identificam o produto se forem a **primeira** palavra do
nome ("Rolo de Lã 23cm" é, aí sim, um rolo de pintura).

⚠️ **Estas fotos são dado de teste.** São genéricas da categoria ("uma
furadeira"), não do produto exato ("Bosch GSB 13 RE"). A foto real vem do
fornecedor (que costuma liberar mídia para revenda) ou da própria loja.

---

## S3 — o que falta

O driver local está implementado e funcionando. Para produção, falta:

- [ ] Bucket na conta AWS da Utilar (a conta ainda não existe)
- [ ] Credencial / role de acesso
- [ ] Decidir **acesso público direto vs. URL assinada** — para foto de produto
      em vitrine, público com CDN na frente é o normal
- [ ] CloudFront (ou equivalente) e o `MEDIA_BASE_URL` apontando para ele
- [ ] Política de ciclo de vida, se fizer sentido

Nada disso foi configurado e **nenhuma credencial foi inventada**. Ver
[`SEPARATION-utilar-vs-gifthy.md`](SEPARATION-utilar-vs-gifthy.md): a conta AWS
tem que ser da Utilar.

---

## Onde olhar no código

| | |
|---|---|
| Detecção de formato | `internal/imaging/sniff.go` |
| EXIF e rotação | `internal/imaging/exif.go` |
| Normalização e variantes | `internal/imaging/normalize.go` |
| Handler de upload | `internal/handler/product_image_upload.go` |
| Schema | `migrations/012_product_image_variants.up.sql` |
