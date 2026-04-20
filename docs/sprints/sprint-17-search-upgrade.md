# Sprint 17 — Upgrade de busca (Meilisearch)

**Fase**: 5 — Crescimento. **Estimativa**: 6–8 dias.

## Escopo

A busca do Sprint 05 usa Postgres `ILIKE` com índice GIN `pg_trgm`. Funciona bem para alguns milhares de SKUs, mas tem dois pontos de ruptura conhecidos: tolerância a erros de digitação (clientes que digitam "furadera" ou "parafuzadeira" não encontram nada) e latência no p95 na escala do catálogo. Condições para executar este sprint: catálogo ultrapassa **10.000 SKUs** OU latência de busca no p95 excede **500 ms** em produção. Este sprint substitui o caminho de busca Postgres pelo **Meilisearch** self-hosted por trás de um adapter, mantendo a API HTTP inalterada e tornando o backend intercambiável via variável de ambiente.

Referência: [ADR 004](../adr/004-search-strategy.md).

## Tarefas

### infrastructure
1. Adicionar container `meilisearch` ao `infrastructure/prod/docker-compose.yml` (v1.5+); montar volume nomeado em `/meili_data`; expor `MEILI_MASTER_KEY` via env; política de restart `unless-stopped`
2. Espelhar o container em `infrastructure/local/docker-compose.yml` com `MEILI_ENV=development` sem exigência de master key
3. Documentar o procedimento de backup do volume de dados do Meilisearch em `infrastructure/README.md` (snapshot noturno + cópia fora do host)
4. Script de health check `infrastructure/prod/scripts/meilisearch-health.sh` consultando `/health` e alertando no Freshdesk em caso de falha (integração do Sprint 16)

### product-service
5. Adapter `services/product-service/app/search/adapter.rb` (interface abstrata: `#search(query, filters, page, per_page)`, `#index(product)`, `#remove(product_id)`, `#reindex_all`)
6. `services/product-service/app/search/pg_adapter.rb` — comportamento atual de ILIKE + pg_trgm movido para trás da interface
7. `services/product-service/app/search/meilisearch_adapter.rb` — usa a gem `meilisearch-ruby`; configura as definições por índice (atributos pesquisáveis, atributos filtráveis, regras de ranking com tokenizador BR-PT)
8. Configuração: ler a env `SEARCH_BACKEND` (`pg` | `meilisearch`, padrão `pg`) na inicialização e selecionar o adapter
9. Hooks de sincronização: `after_commit` no model Product chama `SearchAdapter.current.index(self)` no create/update e `.remove(id)` no destroy
10. Worker de sincronização em background `services/product-service/app/workers/search_sync_worker.rb` reindexando em lotes de 500 quando os hooks estão desabilitados (ex.: durante importação em lote do Sprint 11)
11. Rake task `lib/tasks/search.rake` com `search:reindex` (rebuild completo) e `search:status` (contagem de documentos + timestamp da última atualização)
12. Arquivo de sinônimos `services/product-service/config/search_synonyms_pt_br.yml` semeado com aliases do setor de ferragens:
    - `furadeira ↔ parafusadeira ↔ drill`
    - `chave-de-fenda ↔ chave philips ↔ chave estrela`
    - `martelo ↔ marreta` (com limitação de contexto)
    - `serra circular ↔ serra mármore`
    - ~40 entradas adicionais curadas com especialista do domínio
13. Ajuste de tolerância a erros: padrão de 1 erro para palavras com ≥ 5 caracteres, 2 erros para palavras com ≥ 9 caracteres; desabilitar erros em tokens com formato de SKU (regex: `^[A-Z0-9\-]{4,}$`)

### Gateway
14. Sem alterações — o roteamento de `/api/v1/marketplace/products` permanece; a troca de adapter é interna ao product-service

### SPA (utilar-ferragem)
15. Adicionar componente de autocomplete/sugestão `SearchAutocomplete.tsx` usando o mesmo endpoint `?q=` com debounce de 200ms; exibir as 8 melhores correspondências com categoria + prévia de preço
16. Destacar tokens correspondentes na lista de sugestões usando o campo `_formatted` da resposta do Meilisearch (silenciosamente ignorado no backend `pg`)
17. Dica "Você quis dizer X?" quando a resposta indica uma correspondência com correção de typo (campo preenchido apenas quando `SEARCH_BACKEND=meilisearch`)

## Critérios de aceite
- [ ] A busca por `furadera` retorna resultados contendo `furadeira` (tolerância a erros)
- [ ] A busca por `chave philips` retorna produtos `chave-de-fenda` via o arquivo de sinônimos
- [ ] Latência de busca no p95 abaixo de 150 ms em um índice de 50 mil produtos (medido via Prometheus ou k6)
- [ ] Alternar `SEARCH_BACKEND=pg ↔ meilisearch` requer apenas mudança de env + reinicialização do serviço — sem mudança de código, sem migração de dados
- [ ] Reindexação completa de 50.000 produtos conclui em menos de 5 minutos
- [ ] Autocomplete renderiza sugestões em menos de 200 ms com cache aquecido
- [ ] A rake task `search:status` retorna contagens precisas de documentos compatíveis com o número de linhas da tabela `products` (após reindexação)
- [ ] Criação/atualização de produto se propaga para o índice em até 2 segundos

## Dependências
- Sprint 05 (busca inicial) concluído
- [ADR 004](../adr/004-search-strategy.md) aprovado
- Orçamento de memória do host para o container Meilisearch (≥ 1 GB de RAM por 100k documentos)
- Pipeline de Prometheus / métricas disponível para confirmar o critério de aceite de p95

## Riscos
- Drift do índice (hook falhando silenciosamente) — mitigação: reindexação completa noturna + alarme de drift em `search:status` comparando contagens de documentos
- Qualidade do stemming em BR-PT depende do provedor — curar a lista de sinônimos agressivamente para o lançamento e iterar semanalmente com base nos logs de buscas sem resultado
- Carga operacional do self-hosting — documentar backup, upgrade e procedimentos de recuperação em `infrastructure/README.md` antes da virada
- Meilisearch em nó único é um SPOF — aceitável para a Fase 5; planejar um par com réplica na Fase 6 se a busca se tornar crítica para a receita
- Migrar para Meilisearch durante horário de tráfego — disponibilizar a flag de env primeiro, fazer dark-read via Meilisearch em paralelo por uma semana antes de torná-lo primário
