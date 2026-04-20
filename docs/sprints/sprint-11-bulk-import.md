# Sprint 11 — Importação em lote de SKUs (CSV)

**Fase**: 4 — Operações do vendedor. **Estimativa**: 6–8 dias.

## Escopo

Vendedores de ferragens costumam ter centenas ou até milhares de SKUs entre fixadores, ferramentas elétricas e consumíveis. O formulário de cadastro individual atual é inviável para o onboarding de uma ferragem de médio porte. Este sprint adiciona um fluxo de importação em lote via CSV: o vendedor envia um CSV com o conjunto de colunas canônico abaixo, o product-service valida linha a linha, enfileira as buscas de imagens e confirma as linhas válidas enquanto expõe os erros por linha para as inválidas.

Colunas canônicas do CSV: `sku, name, description, category_slug, price, currency, stock, specs_json, image_urls, weight_kg, dimensions_cm`. As URLs de imagem são separadas por pipe; `specs_json` é um blob JSON compatível com o formato `products.specs` do Sprint 04; `category_slug` é resolvido contra a taxonomia de ferragens introduzida no Sprint 03.

## Tarefas

### product-service
1. Migration `services/product-service/db/migrate/YYYYMMDD_create_import_jobs.rb`: tabela `import_jobs` (`id, seller_id, filename, status ∈ pending|running|completed|failed, total_rows, succeeded_rows, failed_rows, error_report_url, created_at, finished_at`)
2. Migration `services/product-service/db/migrate/YYYYMMDD_create_import_job_rows.rb`: tabela `import_job_rows` (`id, import_job_id, row_number, raw_payload_json, status ∈ queued|succeeded|failed, product_id (nullable), errors_json, created_at`)
3. Adicionar Sidekiq ao Gemfile; montar o Sidekiq em `services/product-service/config/routes.rb` em `/sidekiq` (somente admin)
4. Endpoint `POST /api/v1/products/bulk-import` em `services/product-service/app/controllers/api/v1/products_controller.rb#bulk_import` — aceita `multipart/form-data` com o CSV; retorna `{ job_id }`; valida tamanho ≤ 10 MB e ≤ 5000 linhas
5. Endpoint `GET /api/v1/products/bulk-import/:job_id` — retorna progresso do job + contagens + erros por linha (paginado)
6. Endpoint `GET /api/v1/products/bulk-import/:job_id/errors.csv` — transmite o relatório de erros por linha como CSV para reenvio após correções
7. Worker `services/product-service/app/workers/bulk_import_worker.rb` — faz parse do CSV com `encoding: 'bom|utf-8'` explícito, com fallback para Windows-1252 quando o BOM está ausente; resolve `category_slug → id`; valida preços e moeda contra `%w[USD BRL]`; grava o resultado de cada linha em `import_job_rows`
8. Worker `services/product-service/app/workers/image_fetch_worker.rb` — para cada entrada em `image_urls`, faz fetch com timeout de 5s, envia para o S3 (reutiliza o caminho de upload de imagens existente), atualiza `product.images`; marca a linha como falha se houver erro em alguma imagem
9. Endpoint de download do template CSV `GET /api/v1/products/bulk-import/template.csv` — retorna a linha de cabeçalho canônica + uma linha de exemplo

### Gateway
10. Rotear `/api/v1/products/bulk-import*` para o product-service (o roteamento existente de `/api/v1/products/*` já deve cobrir — verificar se o corpo multipart passa íntegro)

### infrastructure/local
11. Adicionar o serviço Redis ao `infrastructure/local/docker-compose.yml` (reutilizar o Redis existente do user-service se a porta for compartilhada); adicionar `SIDEKIQ_REDIS_URL` ao env do product-service
12. Adicionar um container `product-service-sidekiq` ao compose executando `bundle exec sidekiq`

### SPA (utilar-ferragem)
13. Página `/vendedor/produtos/importar` → `ImportProductsPage.tsx`: dropzone (arrastar e soltar + clique para selecionar), link para download do template CSV, botão "Enviar CSV"
14. Após upload bem-sucedido, a SPA consulta `GET /bulk-import/:job_id` a cada 2s; exibe barra de progresso + tabela de erros por linha (número da linha, coluna, mensagem de erro) conforme os erros chegam
15. Botão "Baixar relatório de erros" conectado ao endpoint `errors.csv`
16. Card de resumo de sucesso: "X produtos importados, Y falharam" com link de volta ao catálogo

## Critérios de aceite
- [ ] Um CSV de 500 linhas importa em menos de 60 segundos (excluindo buscas de imagens)
- [ ] Os erros exibem número da linha + coluna com problema + mensagem legível (pt-BR)
- [ ] As imagens informadas em `image_urls` chegam de fato ao S3 e ficam associadas ao produto correto
- [ ] Sucesso parcial: as linhas válidas são confirmadas; as inválidas não bloqueiam o lote
- [ ] UTF-8 com BOM, UTF-8 sem BOM e Windows-1252 são todos parseados corretamente
- [ ] O template CSV baixado pela SPA abre normalmente no Excel e no LibreOffice com acentos pt-BR intactos
- [ ] Uma linha com `category_slug` inválido falha com mensagem específica (não um erro genérico 500)

## Dependências
- Sprint 10 (onboarding de vendedores) concluído — os vendedores precisam existir para serem donos dos SKUs importados
- Redis + Sidekiq adicionados ao `infrastructure/local/docker-compose.yml`
- Pipeline de upload de imagens para S3 (LocalStack em desenvolvimento) funcionando para uploads de imagens de produto individuais

## Riscos
- Buscas de URLs de imagem lentas ou instáveis — limitar em 5s de timeout por imagem e marcar a linha como falha em vez de travar o job inteiro
- Variação de encoding dos CSVs (Excel no Windows salva CP-1252 por padrão) — detecção explícita de BOM + fallback; documentar isso no template
- Uploads muito grandes — aplicar limite de 10 MB / 5000 linhas tanto no controller quanto no `client_max_body_size` do nginx
- Pressão de memória no Redis do Sidekiq se um vendedor fizer upload diariamente — adicionar job noturno para purgar `import_job_rows` com mais de 30 dias
