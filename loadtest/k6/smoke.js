// Smoke — 1 VU, valida que os endpoints-chave respondem 200. Gate rápido de CI.
//   k6 run loadtest/k6/smoke.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { CATALOG, AUTH, ORDER, pick, CATEGORIES, login } from './lib/config.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    checks: ['rate==1.0'], // no smoke, tudo tem que passar
    http_req_failed: ['rate==0.0'],
  },
};

export default function () {
  check(http.get(`${CATALOG}/health`, { tags: { name: 'catalog/health' } }), {
    'catalog health 200': (r) => r.status === 200,
  });
  check(http.get(`${CATALOG}/api/v1/categories`, { tags: { name: 'categories' } }), {
    'categories 200': (r) => r.status === 200,
  });
  check(http.get(`${CATALOG}/api/v1/products?per_page=12`, { tags: { name: 'products' } }), {
    'products 200': (r) => r.status === 200,
    'products tem data': (r) => (r.json('data') || []).length > 0,
  });
  check(
    http.get(`${CATALOG}/api/v1/products/facets?category=${pick(CATEGORIES)}`, { tags: { name: 'facets' } }),
    { 'facets 200': (r) => r.status === 200 }
  );

  const token = login();
  check(null, { 'login retornou token': () => token !== '' });
  if (token) {
    const res = http.get(`${ORDER}/api/v1/orders?per_page=5`, {
      headers: { Authorization: `Bearer ${token}` },
      tags: { name: 'orders' },
    });
    check(res, { 'orders 200': (r) => r.status === 200 });
  }
  sleep(0.5);
}
