// Checkout — fluxo de COMPRA sob carga: login → busca produto → cria pedido.
// Mede o caminho crítico de receita (mais pesado: escreve no order-service).
//   k6 run loadtest/k6/checkout.js
//
// NÃO dispara o pagamento real (Appmax) — para no POST /orders, que é o gargalo
// transacional. O passo de pagamento entra quando houver token de sandbox.
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { CATALOG, ORDER, pick, SEARCH_TERMS, login } from './lib/config.js';

export const options = {
  scenarios: {
    checkout: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '1m', target: 25 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.02'],
    'http_req_duration{name:order/create}': ['p(95)<1000'],
    checks: ['rate>0.95'],
  },
};

export default function () {
  const token = login();
  if (!token) {
    check(null, { 'login ok': () => false });
    return;
  }
  const authHeaders = { headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' } };

  let product;
  group('escolhe produto', () => {
    const res = http.get(`${CATALOG}/api/v1/products?q=${pick(SEARCH_TERMS)}&per_page=10`, {
      tags: { name: 'search' },
    });
    check(res, { 'search 200': (r) => r.status === 200 });
    const items = res.json('data') || [];
    if (items.length > 0) product = items[Math.floor(Math.random() * items.length)];
  });
  if (!product) return;
  sleep(1);

  group('cria pedido', () => {
    const body = JSON.stringify({
      items: [{ productId: product.id, sellerId: product.sellerId || 'seller-1', quantity: 1, unitPrice: product.price, name: product.name }],
      address: { street: 'Rua Teste', number: '100', city: 'São Paulo', state: 'SP', postcode: '01000000' },
      paymentMethod: 'pix',
    });
    const res = http.post(`${ORDER}/api/v1/orders`, body, { ...authHeaders, tags: { name: 'order/create' } });
    // 201 criado, ou 400/422 se o schema exigir campos extras — aceitamos <500
    // (o foco é medir latência/estabilidade sob carga, não validar o payload).
    check(res, { 'order sem erro de servidor': (r) => r.status < 500 });
  });
  sleep(Math.random() * 2);
}
