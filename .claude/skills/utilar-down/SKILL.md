---
name: utilar-down
description: Encerra a stack local do Utilar — a SPA (Vite :5175) e os 5 serviços Go (payment:8090, catalog:8091, order:8092, auth:8093, assistant:8094). Mata por porta (derruba o go run e o binário-filho) e mantém os bancos Docker de pé por padrão; com --infra também para Postgres/Redis/Redpanda. Use quando o usuário pedir para parar/derrubar/encerrar/desligar os serviços, a stack, o ambiente local, ou fazer shutdown do Utilar. Subir de novo com a skill utilar-up.
---

# Encerrar a stack local do Utilar — `utilar-down`

Para a SPA e os 5 serviços Go. Mata **por porta** (não depende de PID salvo) e
derruba o **grupo de processo** inteiro — pega o `go run` e o binário que ele
compila. Os **bancos ficam de pé** por padrão (subir de novo é rápido).

## Como usar

```bash
.claude/skills/utilar-down/down.sh          # serviços + SPA (mantém Docker/DBs)
.claude/skills/utilar-down/down.sh --infra  # também `docker compose down`
```
Ou, comigo, **`/utilar-down`** (acrescente `--infra` se quiser parar o Docker).

## Por que matar por porta

O `go run` compila e executa um binário-filho; matar só o `go run` deixaria o
servidor (o filho, que segura a porta) vivo. O script pega o **PID que escuta a
porta**, sobe pro **grupo de processo** e manda `TERM` (depois `KILL` se
insistir) — encerra pai e filho juntos. Cada porta é conferida no fim.

## Quando usar `--infra`

- **Sem `--infra`** (padrão): encerrou os serviços mas mantém Postgres/Redis/
  Redpanda — ideal entre ciclos de desenvolvimento ou antes de rodar a
  `qa-utilar` (que já isola em banco efêmero e não precisa dos serviços no ar).
- **Com `--infra`**: liberar tudo (RAM/portas) ou reiniciar o Docker limpo. Os
  dados persistem nos volumes do Docker; não apaga banco.

## Relação com as outras skills

- **utilar-up** — sobe tudo de novo.
- **qa-utilar** — também **para** os serviços (8090-8094) antes de rodar a
  pirâmide; se você acabou de rodar a QA, os serviços já estão parados.
