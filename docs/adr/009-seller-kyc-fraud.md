# ADR 009 — KYC de vendedores e controles antifraude

**Status**: Proposto. **Data**: 2026-04-20.

## Contexto

Um marketplace que permite que qualquer CNPJ se cadastre e comece a listar produtos atrai três problemas específicos: produtos falsificados, evasão fiscal e fraude do tipo "pega o dinheiro e some". O e-commerce brasileiro é um alvo bem documentado para todos os três. O fracasso oposto — exigir um KYC manual pesado de cara — mata a aquisição de vendedores antes de termos qualquer vendedor.

Forças em jogo:
- CNPJ é a identidade de vendedor universalmente reconhecida no Brasil (já exigida pelo Sprint 17 pai)
- ReceitaWS e BrasilAPI expõem a consulta de registro público da Receita Federal gratuitamente em baixo volume
- A capacidade de revisão do admin no lançamento é de 1–2 horas/dia de um único operador — não consegue absorver uma alta taxa de falsos positivos
- Fraude do lado do cliente é diferente: cartões roubados, ataques de BIN, abuso de cupons, fraude de velocidade

Depende desta decisão: Fase 4 / Sprint 10 de hardening do cadastro de vendedores; Fase 5 de retenção de repasse e fluxo de disputas.

## Decisão

### Onboarding de vendedores (Sprint 10)

1. **Coleta** — CNPJ + endereço BR completo (já feito no Sprint 17 pai). Mais: `razao_social` declarada pelo vendedor, `nome_fantasia`, nome do contato principal + CPF.
2. **Validação** — Módulo 11 do CNPJ **no cliente e no servidor** (já feito; lembrete de nível de ADR: se você mudar um lado, mude o outro conforme CLAUDE.md).
3. **Enriquecimento** — chamada server-side para a **BrasilAPI** (`https://brasilapi.com.br/api/cnpj/v1/{cnpj}`, primária) com **fallback ReceitaWS** (`https://receitaws.com.br/v1/cnpj/{cnpj}`, secundário). Cache de respostas por 7 dias no Redis para respeitar as cotas do plano gratuito. Buscar:
   - `situacao_cadastral` (deve ser `ATIVA`)
   - `razao_social` (comparar com a declarada pelo vendedor; Levenshtein ≤ 3 = correspondência)
   - `cnae_fiscal` (registrar para futuras regras de restrição por categoria)
   - `data_abertura` (sinalizar CNPJs com < 90 dias para revisão manual — padrão comum de fraude)
4. **Estado inicial** — `seller.status = 'pending_approval'`. O vendedor não pode listar produtos até ser aprovado.
5. **Rejeição automática** (sem consumir tempo do admin):
   - `situacao_cadastral != 'ATIVA'` (INAPTA, BAIXADA, SUSPENSA)
   - CNPJ duplicado já cadastrado
   - Incompatibilidade clara entre `razao_social` e `nome_fantasia` (Levenshtein > 10 em strings normalizadas)
6. **Aprovação automática** — nenhuma no lançamento. Todo vendedor recebe um olhar humano nos primeiros 6 meses.
7. **Revisão manual** — tudo o mais. O admin vê os dados de enriquecimento lado a lado com os dados declarados pelo vendedor no painel admin do gifthy-hub. SLA: 2 dias úteis.

### Fraude do lado do cliente (Sprint 10)

- **Velocidade de cadastro**: 3 cadastros por IP por hora (contador Redis). Bloqueio total no 4º, alerta para ops.
- **Velocidade de pagamento recusado**: 5 pagamentos recusados por usuário em 24h. Bloqueio suave (deve contatar o suporte).
- **Lista de bloqueio de BIN**: manter uma lista de BINs conhecidos por atacar marketplaces brasileiros; atualizada a partir dos logs de fraude.
- **Bloqueio suave por anomalia**: incompatibilidade no padrão do usuário (novo dispositivo + novo endereço + carrinho de alto valor + novo método de pagamento) dispara um bloqueio suave pendente de revisão manual.
- **Aplicação de 3DS**: ativado via PSP (Mercado Pago / PSP a ser escolhido) para pedidos > R$ 500 ou para qualquer primeiro pedido de uma conta nova.

### MEI microempreendedores individuais

MEI (Microempreendedor Individual) é um tipo de CNPJ válido sem `razao_social` no sentido tradicional — a entidade é a própria pessoa física. Aceitar cadastros de MEI com a mesma validação de CNPJ + cruzamento do CPF do titular. Sinalizar para revisão manual no primeiro listing até termos volume de MEI suficiente para automatizar.

### Alternativas comparadas

| Opção | Cota gratuita | Uptime (observado) | Profundidade dos dados | Fallback necessário |
|--------|-----------------|-------------------|------------|-----------------|
| **BrasilAPI (primária)** | ~3 req/s, sem limite diário | ~99,5% | Bom (registro público) | Sim |
| **ReceitaWS (fallback)** | 3 req/min no plano gratuito | ~98% | Bom | Sim |
| Serasa / SPC | API paga, contrato | ~99,9% | Profundo (crédito + score de fraude) | Não |
| Somente manual (sem API) | Infinita | 100% | O que o admin digitar | N/A |
| Sem KYC | N/A | N/A | N/A | N/A |

BrasilAPI primária + ReceitaWS fallback nos dá ~99,9% de uptime efetivo a custo zero no nosso volume.

## Consequências

### Positivo
- Verificação em tempo real da `situacao_cadastral` pega a fraude mais óbvia (CNPJs inativos/suspensos) antes de eles verem o painel do vendedor
- Módulo 11 no cliente e no servidor pega erros de digitação e fabricações triviais
- APIs de plano gratuito custam $0 no lançamento; caminho de upgrade para Serasa/SPC existe se a taxa de fraude justificar
- Fluxo de revisão manual é uma barreira rígida, não um filtro suave — agentes mal-intencionados não conseguem passar sendo pacientes
- Limites de velocidade previnem os padrões de teste de cartão mais comuns sem exigir uma ferramenta antifraude de terceiros

### Negativo
- Indisponibilidade da BrasilAPI / ReceitaWS bloqueia o onboarding de novos vendedores — o cache Redis ajuda, mas não resolve o primeiro cadastro durante uma indisponibilidade. Mitigação: enfileirar o enriquecimento; admitir o vendedor em `pending_approval` mesmo assim; admin vê o botão "enriquecimento falhou, tentar novamente".
- Revisão manual em 2 dias úteis é um ponto de fricção — vendedores nos comparam ao Shopee / ML que fazem onboarding em minutos. Aceitar por segurança; revisitar se a aquisição sofrer.
- Sem verificação com bureau de crédito = sem visibilidade do histórico de fraude do vendedor em outras plataformas. O upgrade para Serasa fecha essa lacuna, com custo.
- Limites de velocidade têm falsos positivos (IPs de escritório com NAT, blocos de IP residencial compartilhado). Aceitar; suporte pode levantar manualmente os bloqueios.

### Alternativas rejeitadas
- **Serasa / SPC no lançamento**: pago por consulta (precificação por contrato), overhead de onboarding, negociação de DPA para LGPD. Boa ferramenta, sequenciamento errado.
- **Sem KYC além da validade do CNPJ**: abre espaço para listings de produtos falsificados e vendedores com evasão fiscal. Se recuperar de um "escândalo de furadeira falsa" público é muito mais caro do que a revisão front-loaded.
- **Aprovação automática com enriquecimento limpo**: tentador e pode vir depois, mas no lançamento não confiamos suficientemente nas nossas heurísticas. Humano no loop fica por pelo menos 6 meses.
- **KYC terceirizado (Idwall, Unico, etc.)**: pesado para o nosso volume de vendedores; revisitar na Fase 5+ se a revisão manual virar o gargalo.

## Questões em aberto

1. **Verificação de Nota Fiscal Eletrônica (NF-e)** — exigimos que os vendedores emitam NF-e para cada pedido e verificamos a emissão via SEFAZ? Crítico para conformidade fiscal, mas operacionalmente pesado. Defer para a Fase 5; sinalizar como lacuna conhecida para assessoria tributária.
2. **Retenção de repasse durante disputa** — quando um cliente abre um chargeback, congelamos os repasses pendentes do vendedor até a resolução. Quantos dias, quanto, política de liberação automática? Escopo da Fase 5.
3. **Verificação de identidade MEI** — donos de MEI frequentemente não têm uma `razao_social` formal; o enriquecimento retorna apenas o nome da pessoa física. Exigimos selfie + upload de documento CPF para MEI (estilo Unico/Idwall)? Em aberto; decidir após os primeiros 100 cadastros de MEI.
4. **Detecção de produtos falsificados** — além do KYC do vendedor, como identificamos um CNPJ legítimo listando ferramentas DeWalt falsas? Hash de imagem + registro de propriedade de marca (INPI) + denúncias de clientes. Fase 5+.
5. **Retenção de dados** — a LGPD determina que minimizemos a retenção de PII. Cache de enriquecimento em 7 dias, notas de admin por retenção do pedido + 5 anos. Formalizar política com jurídico antes do go-live.
6. **Cadência de reverificação** — um CNPJ que estava ATIVA no cadastro pode se tornar BAIXADA meses depois. Cron mensal re-verifica todos os vendedores ativos; suspender + e-mail em caso de mudança de status. Implementar no Sprint 10 ou deferir?
