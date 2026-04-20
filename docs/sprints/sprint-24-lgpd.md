# Sprint 24 — Conformidade com a LGPD

**Fase**: 3 — Commerce (gate pré-lançamento). **Estimativa**: 4–6 dias.

## Escopo

A LGPD (Lei Geral de Proteção de Dados, equivalente brasileiro ao GDPR) é **obrigatória antes do lançamento público**. Este sprint entrega as superfícies exigidas: **banner de consentimento**, **política de privacidade**, **exportação de dados** (LGPD Art. 18 — portabilidade), **exclusão de conta** (LGPD Art. 18 — apagamento), **minimização de PII nos logs** e documentação de **finalidade limitada de tratamento**.

Um Encarregado de Dados (DPO) é nomeado — **o fundador** para o MVP, listado na página de Privacidade com `dpo@utilarferragem.com.br`. Também documentamos a tensão entre o direito ao apagamento e a obrigação de retenção fiscal de 5 anos (a Receita Federal exige registros de pedidos por 5 anos) — a exclusão é implementada como **pseudonimização**, não como deleção definitiva, nos registros vinculados a documentos fiscais.

Uma revisão jurídica dos Termos de Uso + Privacidade está agendada como dependência bloqueante antes do lançamento — reservar 1 semana para isso.

## Tarefas

### SPA — banner de consentimento
1. Componente `ConsentBanner` (`utilar-web/src/components/consent/ConsentBanner.tsx`) — aparece na primeira visita, PT-BR por padrão, 3 botões: "Aceitar tudo" / "Apenas essenciais" / "Personalizar"
2. Preferências armazenadas em `localStorage.utilar.consent` como `{ essential: true, analytics: bool, marketing: bool, ts: iso }`
3. Hook `useConsent()` lê as preferências; GA4 + pixels de marketing só disparam quando `analytics === true` (integrado no Sprint 25)
4. Re-exibição após 12 meses (boa prática LGPD)
5. Página de configurações `/conta/privacidade` permite ao usuário atualizar suas preferências a qualquer momento

### SPA — páginas legais
6. `PrivacyPolicyPage` em `/privacidade` — lista os dados coletados, finalidades do tratamento (base legal do Art. 7 por finalidade), prazos de retenção, direitos dos usuários, contato do DPO. Rascunho em PT-BR baseado em modelo validado juridicamente; bloqueado na revisão do advogado antes da publicação.
7. `CookiePolicyPage` em `/cookies` — enumera cada cookie / chave localStorage definida, sua finalidade e TTL
8. `LgpdPage` em `/lgpd` — resumo dos direitos do usuário sob o Art. 18, links de autoatendimento para os fluxos de exportação + exclusão
9. `TermsOfServicePage` em `/termos` — Termos de Uso padrão de marketplace, também bloqueado na revisão do advogado
10. Links no rodapé para todas as páginas acima

### user-service — exportação de dados (Art. 18)
11. `POST /api/v1/users/me/export` (JWT) — enfileira um background job; retorna `{ request_id, status: 'processing' }`
12. `UserDataExportJob` agrega dados de todos os serviços: perfil (user-service), registro de vendedor (user-service), pedidos + endereços (order-service), avaliações (review-service), inscrições push (user-service), eventos (order-service `product_events` filtrado por user_id). Saída: zip com um arquivo JSON por conjunto de dados + um `README.txt` explicando o schema.
13. Agregação entre serviços via chamadas ao gateway com um token de serviço interno (não exposto externamente); falha graciosamente se um serviço estiver indisponível (registra no README.txt quais seções estão ausentes)
14. Upload do zip para um bucket S3 privado `utilar-exports`, gera uma URL pré-assinada com validade de 24 horas, envia por e-mail ao usuário via SES
15. `GET /api/v1/users/me/export/:request_id` — consulta de status

### user-service — exclusão de conta (Art. 18)
16. `DELETE /api/v1/users/me` (JWT) — aciona `UserDeletionJob`; a resposta é uma página de confirmação clara em PT-BR avisando sobre a retenção fiscal
17. `UserDeletionJob`:
    - Pseudonimizar `users.email` → `deleted-{id}@utilarferragem.local`, `name` → `Usuário removido`, `cpf` → null
    - Pseudonimizar `sellers.cnpj` → **mantido** (obrigação fiscal — documentado na política de privacidade)
    - Registros de `orders` mantidos com campos de cliente pseudonimizados por 5 anos; itens permanecem intactos
    - Deleção definitiva: push_subscriptions, tokens de sessão, endereços salvos (exceto os referenciados por pedido não enviado), carrinho, visto recentemente, avaliações (ou anonimizar com `Cliente Utilar`)
    - Escrever linha em `deletion_log` com `original_user_id, deleted_at, categories_pseudonymized, categories_hard_deleted`
18. Revogar JWTs (adicionar user_id a uma lista de revogação no Redis) para deslogar sessões ativas imediatamente

### Minimização de armazenamento do CPF
19. Auditar onde o CPF está sendo armazenado: apenas em `orders.customer_cpf` (necessário para nota fiscal). Remover quaisquer cópias incidentais.
20. Armazenar CPF criptografado em repouso com `attr_encrypted` (ou `Lockbox`) usando uma chave do Secrets Manager; adicionar coluna de lookup com hash `customer_cpf_fingerprint` (SHA-256 + pepper) para consultas de deduplicação sem descriptografar
21. Redação nos logs: estender o middleware `PiiRedactor` do Sprint 22 para mascarar CPF como `***.***.***-XX` em todos os logs
22. UI do admin: exibir CPF apenas ao clicar explicitamente em "Revelar", auditar o evento de revelação

### Tabela de auditoria
23. Migration `create_data_access_logs`: `actor_user_id, target_user_id, action (export|delete|admin_view_pii), metadata (jsonb), occurred_at`
24. Exportação noturna deste log para um bucket S3 separado (trilha de conformidade, retenção de 5 anos)

## Critérios de aceite

- [ ] Banner de consentimento aparece na primeira visita em PT-BR; escolha persiste; reaparece após 12 meses
- [ ] "Apenas essenciais" bloqueia o carregamento do GA4 (verificado na aba Network)
- [ ] Páginas `/privacidade`, `/cookies`, `/lgpd`, `/termos` renderizam; todas estão vinculadas no rodapé
- [ ] `POST /api/v1/users/me/export` entrega um zip via e-mail SES em até 60 segundos para um usuário típico
- [ ] Zip contém perfil, pedidos, avaliações, inscrições push como arquivos JSON separados + `README.txt`
- [ ] `DELETE /api/v1/users/me` pseudonimiza a linha do usuário, mantém pedidos (para fins fiscais), revoga JWTs ativos, registra a ação
- [ ] Usuário excluído não consegue fazer login; seus pedidos ainda aparecem na supervisão do admin com `Usuário removido` como nome do cliente
- [ ] CPF aparece como `***.***.***-XX` em toda linha de log em uma auditoria de grep de 100 requisições
- [ ] CPF em repouso está criptografado (verificado por dump do `psql` mostrando ciphertext, não texto plano)
- [ ] Log de acesso a dados registra toda ação de revelação de PII pelo admin
- [ ] Aprovação do advogado sobre Privacidade + Termos arquivada em `docs/legal/`

## Dependências

- Sprint 07 (auth) concluído
- Todos os serviços com PII conhecidos e inventariados
- Sprint 22 (middleware de redação de logs) — estende seus padrões de PII aqui
- Contratação do advogado confirmada para revisão (prazo de 1 semana)
- SES fora do sandbox (Sprint 25) — aceitável entregar exportações via Mailtrap em staging até lá

## Riscos

- Revisão jurídica é o caminho crítico — contratar o advogado no início do sprint; se ele atrasar, o lançamento atrasa
- Retenção fiscal (5 anos) vs. direito ao apagamento — documentar a tensão explicitamente na Privacidade; a abordagem de pseudonimização é padrão do setor, mas deve ser explicada claramente ao usuário na caixa de diálogo de confirmação de exclusão
- Zip de exportação pode ser muito grande para usuários intensivos — limitar a 100MB; se ultrapassado, dividir em múltiplos zips (raro no primeiro ano)
- Banner de consentimento que prejudica o analytics — testar com cuidado; oferecer um fluxo "Personalizar" para não perder todas as aceitações de analytics para um modal assustador de "Aceitar tudo"
