# ADR 004 — Estratégia de busca e filtragem

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

A Utilar Ferragem é um marketplace de ferragens e ferramentas onde os clientes buscam por nome de produto, SKU, marca, categoria e valores de especificação ("furadeira 18V", "parafuso M6 cabeça sextavada", "broca SDS Plus"). O catálogo de lançamento é de ~2–5k SKUs; a meta em 12 meses é 30–50k SKUs distribuídos entre dezenas de vendedores. A filtragem ocorre tanto em colunas de primeira classe (`category`, `brand`, `price`, `currency`) quanto em chaves JSONB de especificações (`specs.voltage`, `specs.diameter_mm`).

Forças em jogo:
- Tokenização em português do Brasil é importante (stemming "furadeiras" → "furadeira", diacríticos "ação" → "acao", sinônimos como "parafusadeira" ≈ "aparafusadeira")
- Tolerância a erros de digitação é essencial — clientes não soletram nomes de ferramentas com precisão
- Não queremos operar e monitorar um novo serviço stateful no dia 1
- O caminho de busca deve permanecer intercambiável para que possamos evoluir sem alterar os componentes de página

O entregável da Sprint 05 depende disto: lista de produtos + painel de facetas no storefront.

## Decisão

**Começar com Postgres** — `ILIKE` para match de prefixo, extensão `pg_trgm` para tolerância a erros de digitação, índice GIN em uma coluna `tsvector` com o dicionário `portuguese` para full-text. Esconder a implementação por trás de uma interface `SearchAdapter` no `product-service` para que a página nunca fale com SQL diretamente.

**Promover para Meilisearch** (container self-hosted no Docker Compose / stack ECS existente) quando qualquer um dos gatilhos disparar:
1. Catálogo ultrapassa **10k SKUs ativos**, OU
2. Latência de busca p95 > **500ms** sob tráfego realista (medido via `PgHero` + logs de acesso do gateway)

A troca é um PR substituindo a implementação do `SearchAdapter` mais um job de reindexação; componentes de página, hooks do TanStack Query e contratos de API não mudam.

### Alternativas comparadas

| Opção | p95 @ 10k | p95 @ 50k | Custo mensal | Carga operacional | Tokenização PT-BR | Tolerância a erros |
|-------|-----------|-----------|--------------|-------------------|-------------------|--------------------|
| **Postgres (ILIKE + pg_trgm + tsvector)** | 200–400ms | 800ms–2s | $0 (RDS existente) | Nenhuma (já operamos) | Dicionário `portuguese` — razoável | Similaridade `pg_trgm` — razoável |
| **Meilisearch (self-hosted)** | 20–60ms | 30–80ms | ~$15/mês (t4g.small) | Baixa (binário único, backups por snapshot) | Nativo, forte | Melhor da classe, zero config |
| Algolia | 10–40ms | 10–40ms | $500+/mês com 50k SKUs + 100k buscas | Zero | Forte | Melhor da classe |
| Elasticsearch | 30–100ms | 50–150ms | ~$80+/mês (gerenciado) ou dor self-host | Alta (tuning de JVM, evolução de mappings, ops de cluster) | Forte (com plugins) | Bom |
| Typesense | 20–60ms | 30–80ms | ~$15/mês self-host | Baixa | OK | Bom |

### Interface SearchAdapter

```ruby
# services/product-service/app/search/search_adapter.rb
class SearchAdapter
  def search(query:, filters:, page:, per_page:, sort:); end
  def facets(query:, filters:); end
  def index(product); end
  def remove(product_id); end
end
```

Duas implementações chegam na Sprint 05: `PgSearchAdapter` (padrão) e um `MeilisearchAdapter` stub (mantido verde no CI, ativado na promoção).

## Consequências

### Positivas
- Zero nova infraestrutura no lançamento — Postgres já está rodando, indexado, com backup
- A promoção é um PR, não uma reescrita; a troca fica atrás da feature flag `SEARCH_ADAPTER=meilisearch`
- Evita lock-in prematuro de fornecedor; só pagamos por um motor de busca quando o catálogo justificar
- A interface `SearchAdapter` também serve como contrato para futuros testes A/B (ex: Meilisearch vs. pg puro para consultas de usuários avançados)

### Negativas
- Full-text + trigram do Postgres é visivelmente mais fraco que Meilisearch para erros de digitação e matches fuzzy — os clientes vão perceber após ~10k SKUs
- Manter a coluna `tsvector` requer uma trigger ou callback ao salvar o `Product`; uma peça móvel a mais
- Quando promovermos, ainda precisaremos de um job de reindexação, arquivo de sinônimos e dashboard de alertas — esse trabalho está adiado, não evitado

### Alternativas rejeitadas
- **Algolia**: o preço escala agressivamente com o tamanho do catálogo + volume de busca; com 50k SKUs e tráfego moderado gastaríamos mais por mês do que toda a nossa conta ECS. Ótimo produto, economicamente errado para um marketplace bootstrap.
- **Elasticsearch**: o peso operacional é difícil de justificar para um catálogo que cabe na memória do Postgres. Tuning de JVM, migrações de mapping e saúde do cluster são uma distração do trabalho de produto.
- **Typesense**: tecnicamente comparável ao Meilisearch, mas comunidade menor, menos referências em português do Brasil, ecossistema mais escasso de exemplos. Mantido apenas como contingência.

## Questões em aberto

1. **Ownership do container Meilisearch** — ele vive no mesmo compose stack de `infrastructure/prod/` (mais simples) ou como serviço ECS separado com sua própria task definition (mais limpo)? Responsável: plataforma, decidir no momento da promoção.
2. **Estratégia de reindexação** — rebuild completo no deploy vs. upserts em streaming via tópico Kafka `product.updated`. Provavelmente streaming desde o dia 1 da promoção; responsável: lead do product-service, Sprint que disparar o gatilho.
3. **Arquivo de sinônimos PT-BR** — quem cuida de `furadeira` ≈ `parafusadeira`, `chave inglesa` ≈ `chave ajustável`, `broca` ≈ `mecha` (regional)? Responsável: conteúdo/ops; lista inicial baseada nos 200 termos de busca mais frequentes após a semana de lançamento.
4. **Busca por valor de especificação** — indexamos os valores de `specs.*` no índice de busca principal, ou os mantemos apenas como facetas? Padrão: somente faceta no lançamento; reavaliar após vermos os logs de consulta reais.
5. **Sinal de ranking além do match textual** — damos boost a produtos em estoque, vendedores bem avaliados ou listagens patrocinadas? Adiar para o pós-lançamento, quando tivermos dados de clique.
