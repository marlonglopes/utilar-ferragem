# Sprint 07 — Auth + conta do cliente

**Fase**: 3 — Comércio. **Estimativa**: 4–5 dias.

## Escopo

Contas de clientes. Reaproveitar o fluxo de auth JWT existente do gifthy-hub.

## Tarefas

1. Adicionar o papel `customer` ao user-service (se ainda não for válido). Configurar `POST /auth/register` para atribuir `customer` como padrão a novos usuários quando nenhum papel for especificado.
2. Copiar `authStore.ts` do gifthy-hub e remover a lógica específica de seller/admin
3. Construir a `LoginPage` (`/entrar`): email + senha, estados de erro, link "Esqueci a senha" (stub para placeholder no Sprint 07)
4. Construir a `RegisterPage` (`/cadastro`): nome, email, CPF (com validador de `src/lib/cpf.ts` — seguir o padrão de `cnpj.ts`), telefone, senha, confirmação de senha, checkbox de consentimento LGPD
5. Construir a `ForgotPasswordPage` (`/esqueci-senha`): entrada de email → envia e-mail de redefinição (stub até o mailer existir; no-op com toast de sucesso é aceitável no Sprint 07)
6. Construir a `AccountPage` (`/conta`): perfil, endereços (CRUD com autofill de CEP), stub de formas de pagamento salvas
7. Proteger as rotas `/conta/*` e `/checkout` com `ProtectedRoute`
8. Guarda de papel: sellers/admins que acessam `/conta` são redirecionados para o gifthy-hub com uma mensagem explicativa
9. Após o login, mesclar o carrinho pré-login com o carrinho salvo (se houver)

## Critérios de aceite

- [ ] Um e-mail novo consegue se cadastrar → login automático → cai na home com avatar da conta na navbar
- [ ] Tentar cadastrar com um e-mail já existente exibe um erro claro (sem vazar informação sobre existência do cadastro)
- [ ] A validação de CPF roda client-side e server-side
- [ ] O login persiste após recarregar a página (JWT no localStorage)
- [ ] Logout limpa o JWT + estado exclusivo de cliente (manter o carrinho? — decisão: manter, para que o usuário possa entrar com outra conta)

## Dependências

- Sprint 06 concluído
- user-service aceita o papel `customer`
- Validador `cpf.ts`

## Riscos

- LGPD: checkbox de consentimento explícito obrigatório; registrar o timestamp de consentimento no servidor
- Regras de senha — começar com orientações estilo NIST (≥ 10 caracteres, sem requisitos arbitrários de complexidade)
