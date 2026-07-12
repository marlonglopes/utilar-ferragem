# Testes de carga (k6) — Utilar Ferragem

Carga sobre os serviços Go. Espelha o setup da Gifthy (`loadtest/k6`).

## Rodar

```bash
# smoke (1 VU, gate rápido) — contra a stack local (make dev-full)
k6 run loadtest/k6/smoke.js

# navegação de catálogo (rampa até 50 VUs)
k6 run loadtest/k6/browse.js

# fluxo de compra (login → produto → POST /orders, até 25 VUs)
k6 run loadtest/k6/checkout.js

# stress (arrival-rate até 300 req/s — acha o ponto de quebra)
k6 run loadtest/k6/stress.js
```

## Alvo

Por padrão aponta pra stack **local** (portas 8090-8093). Para produção, todos os
serviços ficam atrás de um host só:

```bash
BASE_URL=https://api.utilarferragem.com.br k6 run loadtest/k6/browse.js
# ou por serviço: CATALOG_URL=… AUTH_URL=… ORDER_URL=…
```

## Scripts

| Script | Cenário | Thresholds |
|---|---|---|
| `smoke.js` | 1 VU, endpoints-chave | checks 100%, 0 falhas |
| `browse.js` | rampa 1→50 VUs, navegação | p95<800ms, erro<1% |
| `checkout.js` | rampa 1→25 VUs, compra | order p95<1s, checks>95% |
| `stress.js` | 50→300 req/s no catálogo | observa degradação (p95<2s, erro<5%) |

`lib/config.js` centraliza URLs (env), usuário de teste do seed e helpers
(`login`, `pick`). O `checkout.js` **não** dispara o pagamento Appmax (para no
`POST /orders`) — o passo de pagamento entra com o token de sandbox.

## Instalar o k6

`brew install k6` / `apt install k6` / binário em <https://k6.io/docs/get-started/installation/>.
Rate-limit do catálogo (`REDIS_URL`) pode barrar o stress — desligue o Redis pra
medir o teto do serviço, ou suba os limites.
