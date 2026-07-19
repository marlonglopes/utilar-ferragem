# SEO da SPA — estado atual, limites e próximos passos

Documenta o que foi implementado no frontend, o que ficou de fora
deliberadamente e o que precisa de decisão do negócio.

## O que existe hoje

| Item | Onde | Status |
|------|------|--------|
| `lang="pt-BR"`, title e description base | `app/index.html` | ✅ |
| Meta tags por rota (title, description, canonical, OG, Twitter) | `app/src/components/seo/Seo.tsx` | ✅ |
| JSON-LD `Product` + `Offer` (preço, moeda, availability do estoque) | `pages/product/ProductDetailPage.tsx` | ✅ |
| JSON-LD `BreadcrumbList` | categoria, produto, categorias | ✅ |
| JSON-LD `Organization` + `WebSite` com `SearchAction` | `pages/home/HomePage.tsx` | ✅ |
| JSON-LD `FAQPage` | `pages/institutional/HelpPage.tsx` | ✅ |
| `robots.txt` | `app/public/robots.txt` | ✅ |
| `sitemap.xml` gerado no build | plugin em `app/vite.config.ts` | ⚠️ só rotas fixas + categorias |
| Prerender / SSG | — | ❌ não feito, ver abaixo |

Os builders de schema ficam em `app/src/lib/seo.ts` e são cobertos por
`app/src/test/seo.test.ts`.

## Origem canônica

`SITE_URL` vem de `VITE_SITE_URL`, com fallback
`https://www.utilarferragem.com.br` — **este domínio é um palpite e precisa ser
confirmado**. Ele aparece em canonical, `og:url`, nos `@id` do JSON-LD e no
`sitemap.xml`. Definir a variável errada no deploy faz o Google indexar URLs que
não existem.

Defina no ambiente de build:

```bash
VITE_SITE_URL=https://www.dominioreal.com.br npm run build
```

E ajuste a linha `Sitemap:` em `app/public/robots.txt`, que hoje está com o host
escrito à mão.

## Sitemap — como estender com produtos

O plugin `utilar-sitemap` em `vite.config.ts` emite as rotas institucionais e as
8 de categoria (16 URLs). **As URLs de produto não entram**, por dois motivos:

1. o `vite.config.ts` roda em Node antes do bundle existir e não tem acesso ao
   catálogo;
2. o catálogo real tem milhares de itens que mudam com o estoque — embutir isso
   num arquivo estático gerado no build significa sitemap desatualizado a cada
   venda.

O caminho correto é o **catalog-service expor `GET /sitemap-produtos.xml`**
paginado (50k URLs por arquivo, limite do protocolo), e um _sitemap index_
referenciar os dois:

```xml
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://.../sitemap.xml</loc></sitemap>
  <sitemap><loc>https://.../sitemap-produtos.xml</loc></sitemap>
</sitemapindex>
```

Ao adicionar uma categoria em `src/lib/taxonomy.ts`, acrescente o slug em
`CATEGORY_SLUGS` no `vite.config.ts` — a lista é duplicada de propósito e não
tem como ser importada de lá.

## Prerender / SSG — recomendação: NÃO fazer agora

A tarefa permitia adicionar prerender via plugin do Vite se fosse barato. **Não
foi feito, deliberadamente.**

O Googlebot executa JavaScript e lê as tags injetadas pelo `react-helmet-async`,
então o essencial (title, description, canonical, JSON-LD com preço e estoque)
já é indexável sem SSG. O ganho de prerender seria em velocidade de indexação e
em crawlers que não executam JS — Bing, e principalmente os previews de
WhatsApp, Facebook e Twitter, que **não executam JS** e hoje mostram apenas o
title e a description estáticos do `index.html` para qualquer link compartilhado.

O motivo de não fazer agora:

- `vite-plugin-prerender` e similares estão sem manutenção ativa e exigem
  Puppeteer no pipeline de build;
- prerender só resolve rotas conhecidas em build time. As páginas que mais
  importam para busca são as de produto, que são dinâmicas — prerender delas
  exige acesso ao catálogo no build, o mesmo problema do sitemap;
- o risco de quebrar o build de produção não compensa o ganho marginal sobre o
  que o helmet já entrega.

**Recomendação:** quando o preview de link em rede social virar prioridade,
resolver no edge (Cloudflare Worker / CloudFront Function) fazendo
_user-agent sniffing_ dos bots de social e servindo um HTML mínimo com as OG
tags corretas, buscadas do catalog-service. É mais barato e mais preciso que
prerender em build, e não acopla o frontend ao catálogo.

## Decisões pendentes

1. **Domínio canônico definitivo** (`VITE_SITE_URL` + `robots.txt`).
2. **Imagem de Open Graph.** Hoje o fallback é `/favicon.svg` — SVG não é
   suportado como preview em rede social. Precisa de um PNG/JPG 1200×630 em
   `app/public/`, e o caminho dele vira o default em `Seo.tsx`.
3. **Perfis sociais** para o campo `sameAs` do schema `Organization`
   (`src/lib/seo.ts`), hoje um array vazio.
4. **Sitemap de produtos** no catalog-service (acima).
