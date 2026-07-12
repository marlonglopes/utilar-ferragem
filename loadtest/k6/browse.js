// Browse — carga de navegação de catálogo (o tráfego dominante de uma loja).
// Rampa até ~50 VUs navegando: home → categoria → busca → detalhe → facets.
//   k6 run loadtest/k6/browse.js
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { CATALOG, pick, CATEGORIES, SEARCH_TERMS } from './lib/config.js';

export const options = {
  scenarios: {
    browse: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 20 },
        { duration: '1m', target: 50 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'], // <1% de erro
    http_req_duration: ['p(95)<800'], // p95 < 800ms
    'http_req_duration{name:products}': ['p(95)<600'],
  },
};

export default function () {
  group('home + categoria', () => {
    check(http.get(`${CATALOG}/api/v1/products?per_page=24`, { tags: { name: 'products' } }), {
      '200': (r) => r.status === 200,
    });
    const cat = pick(CATEGORIES);
    check(
      http.get(`${CATALOG}/api/v1/products?category=${cat}&per_page=24`, { tags: { name: 'products' } }),
      { '200': (r) => r.status === 200 }
    );
    check(http.get(`${CATALOG}/api/v1/products/facets?category=${cat}`, { tags: { name: 'facets' } }), {
      '200': (r) => r.status === 200,
    });
  });
  sleep(Math.random() * 2);

  group('busca + detalhe', () => {
    const res = http.get(`${CATALOG}/api/v1/products?q=${pick(SEARCH_TERMS)}&per_page=12`, {
      tags: { name: 'search' },
    });
    check(res, { '200': (r) => r.status === 200 });
    const items = (res.json('data') || []);
    if (items.length > 0) {
      const slug = items[Math.floor(Math.random() * items.length)].slug;
      check(http.get(`${CATALOG}/api/v1/products/${slug}`, { tags: { name: 'detail' } }), {
        '200': (r) => r.status === 200,
      });
    }
  });
  sleep(Math.random() * 3);
}
