# auth-service

Serviço de identidade do marketplace Utilar Ferragem — registro, login, sessões, emails de verificação, reset de senha, endereços salvos. Emite JWTs consumidos por `order-service` e `payment-service` (mesmo `JWT_SECRET`).

| | |
|---|---|
| **Stack** | Go 1.26 + Gin 1.12 + Postgres 17 + argon2id + JWT HS256 |
| **Porta** | `:8093` |
| **DB** | `utilar_auth_db` (Postgres em `localhost:5438`) |
| **Status** | Phase B3 ✅ em dev; frontend (Login/Register pages + hooks) plugado via `VITE_AUTH_URL` |

Documentação transversal:
- [README raiz](../../README.md)
- [Database maintenance](../../docs/maintenance/database.md)
- [order-service](../order-service/README.md) — consome JWT emitido aqui

---

## Modelo de dados

| Tabela | Propósito |
|---|---|
| `users` | identidade (id, email UNIQUE, password_hash, name, cpf, phone, role, email_verified) |
| `addresses` | endereços salvos do usuário (FK → users, constraint parcial: 1 default por user) |
| `email_verification_tokens` | tokens 1-use para confirmar email (TTL 24h) |
| `password_reset_tokens` | tokens 1-use para reset de senha (TTL 1h) |
| `refresh_tokens` | sessões revogáveis (TTL 30d, 1 linha por login; revoga todas ao reset) |

**ENUM** `user_role`: `customer`, `seller`, `admin`.

Extensões: `pgcrypto` (implícita via `gen_random_uuid()`).

## Segurança

- **Senhas:** argon2id com memory=19MiB, iterations=2, parallelism=1, salt=16 bytes, key=32 bytes (recomendação OWASP 2023). Hash codificado no formato PHC.
- **Tokens:**
  - `accessToken` — JWT HS256 de curta duração (15 min). Sem estado no servidor. Contém `sub` (user_id), `email`, `role`, `exp`, `iat`, `iss`.
  - `refreshToken` — string opaca (UUID v4 sem hífen, 128 bits). Armazenado em `refresh_tokens`. Revogável via `POST /auth/logout`.
- **Ataques endereçados:**
  - Credential enumeration: login devolve 401 genérico para email inexistente E senha errada (mesma mensagem).
  - Forgot password: sempre devolve 200 mesmo se email não existir.
  - Reset de senha: revoga todas as sessões ativas do user.
  - Tokens expirados: rejeitados no parse.
- **Ainda faltando** (Sprint 22/24 / prod):
  - Rate limiting em `/auth/login` e `/auth/forgot-password`.
  - Envio real de email (hoje os tokens são impressos em log via `slog` em dev).
  - Lockout após N logins falhados.
  - HMAC no refresh token (hoje é opaco; funciona, mas uma camada a mais não custa).

## API

Base URL em dev: `http://localhost:8093`. Error envelope `{error, code, requestId}` em todas as falhas.

### Públicos (sem auth)

| Método | Rota | Descrição |
|---|---|---|
| `GET`  | `/health` | liveness probe |
| `POST` | `/api/v1/auth/register` | cria user + emite tokens (ver schema do body abaixo) |
| `POST` | `/api/v1/auth/login` | retorna `accessToken` + `refreshToken` + `user` |
| `POST` | `/api/v1/auth/refresh` | troca `refreshToken` por novo `accessToken` |
| `POST` | `/api/v1/auth/forgot-password` | emite token de reset (log em dev) |
| `POST` | `/api/v1/auth/reset-password` | consome token + troca senha + revoga sessões |
| `POST` | `/api/v1/auth/verify-email` | consome token + marca `email_verified = true` |

### Protegidos (JWT Bearer)

| Método | Rota | Descrição |
|---|---|---|
| `GET`    | `/api/v1/me` | user atual (decodificado do JWT) |
| `POST`   | `/api/v1/auth/logout` | revoga um refresh token específico |
| `GET`    | `/api/v1/addresses` | lista endereços do user |
| `POST`   | `/api/v1/addresses` | cria endereço (opcionalmente marca como default) |
| `DELETE` | `/api/v1/addresses/:id` | remove endereço |

### Exemplo: login

```bash
curl -X POST http://localhost:8093/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test1@utilar.com.br","password":"utilar123"}'
```

Response 200:
```json
{
  "user": {
    "id": "00000000-0000-4000-8000-000000000001",
    "email": "test1@utilar.com.br",
    "name": "Ana Silva",
    "role": "customer",
    "emailVerified": true,
    "createdAt": "2026-04-24T..."
  },
  "accessToken": "eyJhbGciOiJIUzI1NiIs...",
  "refreshToken": "a1b2c3d4..."
}
```

### Exemplo: usar JWT em outro serviço

```bash
TOKEN="eyJhbGciOiJIUzI1NiIs..."
curl -H "Authorization: Bearer $TOKEN" http://localhost:8092/api/v1/orders
```

O `order-service` valida o mesmo JWT_SECRET e extrai `sub` como `user_id`.

---

## Configuração

| Var | Default | Descrição |
|---|---|---|
| `PORT` | `8093` | porta HTTP |
| `AUTH_DB_URL` | `postgres://utilar:utilar@localhost:5438/auth_service?sslmode=disable` | DSN Postgres |
| `JWT_SECRET` | `change-me-in-prod-please` | **precisa ser o mesmo** em auth + order + payment |

Em produção, `JWT_SECRET` virá de AWS Secrets Manager; `AUTH_DB_URL` apontará para RDS.

---

## Rodar

```bash
make infra-up                   # Postgres + Redpanda
make auth-db-reset              # schema + 20 users seed
make auth-run                   # servidor :8093

# atalho: sobe tudo (infra + payment + catalog + order + auth + SPA)
make dev-full
```

### Comandos do Makefile

```bash
make auth-run              # roda o servidor
make auth-build            # compila binário
make auth-test             # 22 testes (5 password + 4 JWT + 13 handlers)

make auth-db-migrate       # aplica *.up.sql
make auth-db-migrate-down  # reverte
make auth-db-seed          # popula 20 users
make auth-db-reset         # down + up + seed
make auth-db-status        # \dt + contagens
make auth-db-psql          # shell interativo
make auth-db-dump          # backups/auth_<ts>.sql
make auth-db-restore FILE=<path>
```

### Ferramenta CLI auxiliar

```bash
go run ./cmd/hash <senha>
# gera um hash argon2id para embutir no seed.sql
```

---

## Testes

22 testes distribuídos em 3 arquivos:

**`internal/auth/password_test.go`** (5 tests)
- Roundtrip hash ↔ verify
- Senha errada rejeitada
- Hash do seed verifica `utilar123` (guarda contra drift)
- Hashes da mesma senha diferem (salt aleatório) mas ambos validam
- Hash inválido rejeitado

**`internal/auth/jwt_test.go`** (4 tests)
- Generate + Parse roundtrip
- Token expirado rejeitado
- Secret errado rejeitado
- Token malformado rejeitado

**`internal/handler/auth_test.go`** (13 tests — integration, require DB seeded)
- Register: sucesso / email duplicado (409) / senha fraca (400)
- Login: sucesso / senha errada (401) / email inexistente (401 genérico)
- Me: com JWT válido / sem token (401) / token inválido (401)
- Refresh: sucesso com refresh do login / token inexistente (401)
- Addresses: list / create + delete

```bash
make auth-test
```

---

## Seed

20 users + 29 endereços. **Senha de todos: `utilar123`**.

```
test1@utilar.com.br    ← use este para smoke test manual (tem 2 endereços: Principal + Trabalho)
test2..test17@...      customer
seller1@utilar.com.br  seller (role)
seller2@utilar.com.br  seller
admin@utilar.com.br    admin
```

O hash argon2id embutido em `seed.sql` valida `utilar123` em qualquer máquina — foi gerado uma vez via `go run ./cmd/hash utilar123` e reutilizado para todos os users (o salt está embutido no encoded).

`test11@utilar.com.br` tem `email_verified = false` propositalmente (para testar fluxo de verify).

---

## Integração com o frontend

Hooks/stores atualizados:
- [app/src/store/authStore.ts](../../app/src/store/authStore.ts) — User agora tem `refreshToken` opcional
- [app/src/lib/api.ts](../../app/src/lib/api.ts) — `authPost`/`authGet` usando `VITE_AUTH_URL`; `isAuthEnabled` flag; helpers `orderXxxWithJWT` para chamar order-service com Authorization Bearer
- [app/src/pages/auth/LoginPage.tsx](../../app/src/pages/auth/LoginPage.tsx) + [RegisterPage.tsx](../../app/src/pages/auth/RegisterPage.tsx) — detectam `isAuthEnabled`, chamam `POST /api/v1/auth/login|register`, setam `user.token = accessToken` + `user.refreshToken`
- [app/src/hooks/useOrders.ts](../../app/src/hooks/useOrders.ts) — prefere JWT quando `isAuthEnabled`, fallback para X-User-Id

Login via mock (sem `VITE_AUTH_URL`) continua aceitando qualquer credencial — útil para dev offline.

---

## Próximos passos

- **Envio de email real** via SES (parte da Sprint 22/25). Hoje os links de verificação / reset aparecem no `slog`.
- **Rate limiting** em `/login` e `/forgot-password` (Sprint 22).
- **2FA** — deferir até necessário; não é bloqueador de lançamento.
- **OAuth providers** (Google / Meta) — deferir.
- **Admin endpoints** (list/search users, disable account) — parte do console admin (Sprint 20).
