# ADR 008 — Estratégia de evolução do schema do banco de dados

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

O catálogo da Utilar está sobre o schema Postgres já existente do product-gateway, que continua evoluindo (veja ADR 002: `products.specs` JSONB, `products.currency`, `users.cpf`). Adicionamos colunas específicas de ferragens ao longo de vários sprints. Mudança de schema é a principal fonte de incidentes em produção em projetos Rails — migrações inseguras bloqueiam tabelas, apagam dados ou quebram código antigo contra o novo schema durante deploys com rolling restart.

Forças em jogo:
- Fazemos deploy com rolling restarts — código antigo e novo rodam lado a lado por segundos ou minutos
- Algumas migrações são baratas (`ADD COLUMN NULL`), outras são bombas-relógio (`ADD COLUMN NOT NULL DEFAULT` em tabelas grandes, `RENAME COLUMN`, `CHANGE TYPE` com cláusula USING)
- `specs` JSONB é genuinamente não estruturado por categoria — um schema rígido não serve
- Migrações sem downtime são o requisito padrão assim que tivermos usuários reais

Depende desta decisão: todo sprint que toca o banco de dados — o que é praticamente todo sprint.

## Decisão

### Regra 1 — Todas as migrações são aditivas por padrão

Novas colunas chegam **nullable, sem default**. O backfill roda de forma assíncrona (task Rake ou job Sidekiq, em lotes). Somente após a conclusão do backfill adicionamos a constraint `NOT NULL` (em um deploy separado).

### Regra 2 — Mudanças destrutivas exigem um deploy em 2 etapas

Renomeações, remoções e mudanças de tipo são feitas em múltiplos deploys:

| Mudança | Passo 1 | Passo 2 | Passo 3 | Passo 4 |
|--------|--------|--------|--------|--------|
| **Renomear coluna** | Adicionar nova coluna + dual-write (callback no model) | Backfill nova a partir da antiga | Migrar leituras para a nova | Remover coluna antiga |
| **Remover coluna** | Parar de escrever no código | Deploy | Remover coluna na próxima migração | — |
| **Mudar tipo** | Adicionar nova coluna com o novo tipo | Dual-write + backfill | Migrar leituras | Remover a antiga |
| **Adicionar NOT NULL** | Adicionar nullable | Backfill | Adicionar constraint (`VALIDATE` separadamente) | — |

### Regra 3 — A gem `strong_migrations` impõe as regras

Instalar [strong_migrations](https://github.com/ankane/strong_migrations) em todo serviço Rails. Operações perigosas falham no CI com uma explicação e uma receita de migração segura. Sobrescritas exigem bloco `safety_assured { ... }` + comentário explicando o motivo — code review pega a preguiça.

Verificações impostas:
- `add_column` com `NOT NULL` + default em tabelas grandes (Postgres 11+ torna isso seguro, mas a gem avisa para padrões antigos)
- `remove_column` (aviso — verificar duas vezes se nada ainda lê a coluna)
- `rename_column`, `rename_table`
- `add_index` sem `algorithm: :concurrently`
- Mudanças de tipo em `change_column` que disparam reescrita de tabela
- Blocos `execute` com `ALTER TABLE` (revisão manual obrigatória)

### Regra 4 — Versionamento de JSONB para `products.specs`

```json
{
  "_schema_version": 2,
  "voltage": "18V",
  "battery_ah": 2.0,
  "chuck_mm": 13
}
```

- Todo JSON de `specs` carrega `_schema_version` (inteiro, começa em 1)
- Métodos adaptadores `Product#specs_v2` / `Product#specs_v3` no model normalizam as leituras
- Writers sempre escrevem na versão mais recente
- Um job de migração em background atualiza as linhas de forma preguiçosa (em lotes) quando cortamos uma nova versão
- A camada de exibição lê pelo adaptador — nunca acessa chaves brutas do JSON diretamente

### Alternativas comparadas

| Abordagem | Segurança | Fricção no desenvolvimento | Manutenção | Notas |
|----------|--------|--------------|-------------|-------|
| **strong_migrations + aditivo por padrão + versionamento JSONB** | Alta | Baixa após criar o hábito | Baixa | Gem é mantida, comunidade Rails brasileira a usa |
| pg_migrate / Rails migrations puras, sem guardrails | Baixa | Zero | Alta (incidentes) | Um rename errado e estamos fora do ar |
| Tabelas por versão de schema (specs_v1, specs_v2) | Alta | Alta (joins em todo lugar) | Alta (N tabelas por categoria) | Excesso de engenharia |
| EAV (entidade-atributo-valor) para specs | Baixa | Alta | Muito alta | Anti-padrão bem documentado |
| Tabelas estruturadas dedicadas por categoria | Média | Muito alta (taxonomia muda constantemente) | Muito alta | Não consegue acompanhar adições de categoria |

## Consequências

### Positivo
- Rolling deploys permanecem seguros — código antigo continua funcionando contra o novo schema até que o Passo N faça a limpeza
- Versionamento de JSONB nos dá flexibilidade de schema sem cair na armadilha do EAV
- `strong_migrations` transforma "o dev sênior sabia melhor" em uma verificação de CI — novos engenheiros não conseguem enviar acidentalmente um bloqueio de tabela de 40 minutos
- Backfills acontecem de forma assíncrona; nunca bloqueiam um deploy

### Negativo
- Mais deploys por mudança lógica — um rename passa de 1 PR para 3 PRs ao longo de 1–2 semanas
- O adaptador de `specs` adiciona indireção — engenheiros precisam resistir a ler `product.specs['voltage']` diretamente
- Atualizações preguiçosas de JSONB significam linhas com versões mistas no banco a qualquer momento; o adaptador precisa lidar com isso de forma elegante
- Avisos do `strong_migrations` podem ser irritantes para tabelas obviamente pequenas; `safety_assured` está disponível mas precisa ser justificado

### Alternativas rejeitadas
- **Tabelas normalizadas por categoria** (ferramentas, fixadores, adesivos cada um com suas próprias colunas): cada nova categoria = migração de schema + model + UI de admin. A taxonomia de ferragens evolui todo mês; nos afogaríamos em migrações.
- **EAV**: anti-padrão clássico. Queries viram N+1 joins, índices são dolorosos, tooling rejeita. A dor está documentada em todos os projetos Rails que tentaram.
- **Sem guardrails**: eventualmente entregaríamos um `remove_column` contra uma coluna ainda referenciada pela réplica antiga e teríamos uma indisponibilidade. Mais barato instalar strong_migrations hoje.
- **Migrações de schema somente no nível da aplicação (sem Rails migrations)**: perde as verificações estruturais (FKs, constraints) que tornam o Postgres confiável.

## Questões em aberto

1. **Quando extraímos campos JSONB quentes para colunas reais?** Heurística: quando uma chave de spec for usada em **>30% das sessões de busca** como filtro, promovê-la a uma coluna de primeira classe + backfill + índice GIN. Candidatas no dia 1: `voltage`, `chuck_mm`, `thread_pitch`. Responsável: líder do product-service, revisão trimestral.
2. **Observabilidade de backfill** — progresso de job Sidekiq para backfills com milhões de linhas: tabela de progresso caseira ou algo como `batched_migrations` da gem `online_migrations`? Decidir antes do primeiro grande backfill.
3. **Coordenação de schema entre serviços** — se o formato de `products.sku` mudar, inventory-service e order-service precisam saber. Padrão: apenas mudanças aditivas nas fronteiras de serviço; coordenar mudanças destrutivas por meio de plano explícito entre serviços + ordem de deploy.
4. **Atualizações de versão do Postgres no RDS** — seguimos o upstream Postgres de forma agressiva (patches de segurança, novos recursos como `pg_ivm`) ou ficamos na versão recomendada pela AWS? Padrão: atualização automática de versão minor ativa; versão major manual, testada em staging.
5. **Meta de recuperação point-in-time** — retenção atual do WAL; 7 dias é suficiente para pegar um bug destrutivo de schema? Tradeoff entre custo e segurança. Responsável: plataforma.
