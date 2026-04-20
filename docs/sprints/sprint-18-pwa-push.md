# Sprint 18 — PWA + notificações web push

**Fase**: 5 — Crescimento. **Estimativa**: 5–7 dias.

## Escopo

Transformar a SPA da Utilar em um PWA instalável e adicionar notificações web push para eventos de status de pedidos. Um service worker faz cache do shell do app e da lista de produtos para que um visitante recorrente possa navegar pelo catálogo offline; pedidos usam network-first com fallback em cache. Quando offline, as páginas são renderizadas a partir do cache e um badge persistente "Sem conexão" aparece no cabeçalho.

As notificações web push se integram às transições de status existentes do order-service: após o usuário optar por receber, uma linha `push_subscription` é armazenada no servidor e a lib de envio distribui os eventos push para todos os endpoints ativos daquele usuário conforme os pedidos avançam no pipeline. A permissão é solicitada em contexto (após a primeira compra), nunca na primeira visita.

## Tarefas

### SPA — shell PWA
1. Adicionar `vite-plugin-pwa` ao `utilar-web/vite.config.ts` no modo `injectManifest` com um service worker personalizado em `utilar-web/src/sw/service-worker.ts`
2. Criar `utilar-web/public/manifest.webmanifest`: `name: "Utilar Ferragem"`, `short_name: "Utilar"`, `theme_color`, `background_color`, `display: standalone`, ícones em 192/512/maskable (SVG fonte em `utilar-web/public/icons/`)
3. Estratégias de cache em runtime no service worker:
   - `stale-while-revalidate` para `/api/v1/marketplace/products*` (catálogo) com `maxEntries: 100, maxAgeSeconds: 3600`
   - `network-first` (timeout 3s) para `/api/v1/orders*`
   - `cache-first` para assets estáticos (`/assets/*`, fontes, ícones)
4. Chaves de cache versionadas a partir de `__BUILD_ID__` injetado no momento do build; `skipWaiting()` + `clientsClaim()` com um toast de prompt ("Nova versão disponível — recarregar?") para evitar bugs de cache obsoleto no deploy
5. Componente `OfflineBadge` em `utilar-web/src/components/layout/OfflineBadge.tsx` — lê `navigator.onLine` + eventos `online`/`offline`, renderiza pill `"Sem conexão"` na topbar
6. Prompt de instalação: capturar `beforeinstallprompt`, guardar o evento, exibir banner dispensável "Adicionar à tela inicial" após 2 sessões

### SPA — UX de assinatura push
7. Componente `NotificationOptIn` (`utilar-web/src/components/notifications/NotificationOptIn.tsx`) — renderizado na página de confirmação de pedido após a primeira compra bem-sucedida do usuário (NÃO na primeira visita)
8. Ao aceitar: solicitar `Notification.permission`, chamar `registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: VAPID_PUBLIC })`, `POST /api/v1/push/subscribe`
9. Fluxo de recusa silenciosa: se o usuário dispensar duas vezes, suprimir por 30 dias (flag `utilar.push.dismissedUntil` no localStorage)
10. Página de configurações (`/conta/notificacoes`): listar assinaturas ativas (por `user_agent`), permitir revogar por dispositivo

### user-service — assinaturas
11. Migration `create_push_subscriptions`: `user_id (FK)`, `endpoint (string, unique)`, `keys_json (jsonb — p256dh + auth)`, `user_agent (string)`, `created_at`, `last_seen_at`
12. `POST /api/v1/push/subscribe` (JWT) — upsert por endpoint
13. `DELETE /api/v1/push/unsubscribe` (JWT) — soft-delete por endpoint ou id
14. Gerar par de chaves VAPID via `rake push:generate_vapid`; armazenar a pública em `VITE_VAPID_PUBLIC_KEY`, a privada nos secrets do user-service

### Lib compartilhada de envio de notificações
15. Gem Ruby ou pasta lib `services/shared/notification_sender/` — encapsula a gem `web-push`; recebe `(user_id, title, body, url, icon)` e distribui para todas as assinaturas ativas; remove endpoints com resposta 404/410 ao falhar no envio
16. Conectar às transições de status do order-service (`paid`, `shipped`, `delivered`, `cancelled`) e aos eventos `payment.confirmed`/`payment.failed` do payment-service
17. Textos de notificação em pt-BR: `"Pedido #1234 confirmado"`, `"Seu pedido saiu para entrega"`, etc. A URL de deep link aponta para `/conta/pedidos/:id`

## Critérios de aceite

- [ ] O prompt "Adicionar à tela inicial" aparece no Android Chrome após a 2ª sessão; o app instalado abre em modo standalone
- [ ] O modo offline renderiza `/` + páginas de categoria a partir do cache em até 500ms; badge "Sem conexão" visível
- [ ] A navegação pelo catálogo funciona offline após uma única visita online anterior
- [ ] A notificação push chega em até 5s após uma transição de status do pedido (testado via `rails c` → flip manual de status)
- [ ] Cancelar assinatura em `/conta/notificacoes` interrompe pushes futuros para aquele dispositivo
- [ ] Instalável no Chrome para desktop (verificação PWA do Lighthouse aprovada)
- [ ] Fluxo de atualização do service worker: o deploy incrementa `__BUILD_ID__`, o usuário vê o toast "Nova versão", o reload carrega os assets novos sem tela em branco

## Dependências

- Sprint 09 (pedidos) concluído — as transições de status emitem eventos que o sender utiliza
- ADR 010 (canais de notificação) aceito
- Par de chaves VAPID gerado e armazenado nos secrets de env

## Riscos

- Suporte parcial a PWA no iOS Safari — web push só está disponível no iOS 16.4+ e exige que o app seja instalado via "Adicionar à Tela de Início". Documentar as limitações em `/ajuda/notificacoes`; manter o e-mail como canal garantido.
- Bugs de cache do service worker no deploy — usar chaves de cache versionadas + `skipWaiting` + toast de "nova versão"; incluir uma rota de kill-switch (`/sw-unregister`) que cancela o registro de todos os service workers em caso de rollout problemático.
- A entrega de push não é garantida (throttling pelo fabricante do browser) — nunca depender exclusivamente de push para confirmações transacionais; sempre espelhar via e-mail.
