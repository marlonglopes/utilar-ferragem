# Sprint 10 — Onboarding de vendedores (vertical de ferragens)

**Fase**: 4 — Vertical do vendedor. **Estimativa**: 7–10 dias.

## Escopo

Transformar o marketplace de um catálogo pré-populado fechado em uma plataforma self-service para negócios de ferragens.

## Tarefas

1. Criar `SellLandingPage` (`/vender`): proposta de valor, transparência de comissões, espaço para depoimentos, FAQ, CTA principal "Cadastrar minha ferragem"
2. Criar `SellerOnboardingWizard` em `/vender/cadastro`, com 6 etapas:
   - **Etapa 1 — CNPJ**: campo de CNPJ com validação via `cnpj.ts`; preenchimento automático opcional a partir de um proxy da Receita Federal (custo: requer API paga ou scraper próprio — padrão: preenchimento manual, auto-fill fica para depois)
   - **Etapa 2 — Perfil**: razão social, nome fantasia, telefone, CEP + endereço (autofill via ViaCEP)
   - **Etapa 3 — Categorias**: seleção múltipla das categorias folha da taxonomia de ferragens (mínimo 1 obrigatória)
   - **Etapa 4 — Comercial**: faixa de comissão, volume mensal estimado, observações iniciais
   - **Etapa 5 — Bancário**: banco, agência, conta, chave PIX (para repasse)
   - **Etapa 6 — Termos**: prévia do contrato, consentimento LGPD, checkbox de assinatura digital
3. Envio do formulário → cria registro em `sellers` com `status=pending`; envia e-mail de boas-vindas; redireciona para uma página "Obrigado — aguardando aprovação"
4. Revisão pelo admin: no painel admin do gifthy-hub, vendedores pendentes aparecem em uma nova fila com ações de aprovação/rejeição (rejeição exige um motivo, enviado por e-mail)
5. Após aprovação: vendedor recebe e-mail com instruções para acessar o gifthy-hub e cadastrar produtos
6. gifthy-hub: exibir banner estilo UTM no primeiro login de novos vendedores apontando para "Cadastre seu primeiro produto"
7. user-service: garantir que `seller.status IN ('approved', 'active')` para que os produtos do vendedor fiquem visíveis nos endpoints públicos (mudança aditiva nos escopos existentes)

## Critérios de aceite

- [ ] Um novo usuário (sem conta existente) consegue concluir o onboarding em até 10 minutos
- [ ] A validação de CNPJ detecta erros de formatação e de dígito verificador
- [ ] O autofill do ViaCEP funciona e tem debounce (sem sobrecarregar o serviço)
- [ ] O admin visualiza o novo vendedor na fila de pendentes em questão de segundos
- [ ] O e-mail de aprovação tem um deep link direto para o login no gifthy-hub
- [ ] Após a aprovação, o primeiro produto cadastrado fica visível na Utilar Ferragem em até 5 minutos

## Dependências

- Fase 3 (lançamento) concluída
- Painel admin do gifthy-hub com suporte à fila de aprovação de vendedores — estender se necessário
- Infraestrutura de e-mail (SES ou equivalente) em funcionamento

## Riscos

- Tamanho do formulário de onboarding — 6 etapas é limite; cortar agressivamente qualquer etapa que não seja legalmente obrigatória no cadastro
- Gargalo na revisão manual pelo admin — ok para os primeiros 50 vendedores; definir um SLA (≤ 2 dias úteis)
- Fraude: um agente mal-intencionado pode criar um CNPJ falso e tentar cadastrar produtos irregulares — mitigar com revisão manual na Fase 4; adicionar KYC automático depois
