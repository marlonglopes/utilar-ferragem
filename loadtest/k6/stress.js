// Stress — rampa agressiva até achar o ponto de quebra do catálogo (endpoint
// mais quente). Sobe além do esperado pra ver onde p95/erros degradam.
//   k6 run loadtest/k6/stress.js
import http from 'k6/http';
import { check } from 'k6';
import { CATALOG, pick, CATEGORIES, SEARCH_TERMS } from './lib/config.js';

export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-arrival-rate',
      startRate: 10,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 300,
      stages: [
        { duration: '30s', target: 50 }, // 50 req/s
        { duration: '1m', target: 150 }, // 150 req/s
        { duration: '1m', target: 300 }, // 300 req/s
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    // Não abortamos — queremos VER a degradação. Marca amarelo se estourar.
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<2000'],
  },
};

export default function () {
  const r = Math.random();
  if (r < 0.5) {
    check(http.get(`${CATALOG}/api/v1/products?category=${pick(CATEGORIES)}&per_page=24`, { tags: { name: 'list' } }), {
      '200': (res) => res.status === 200,
    });
  } else if (r < 0.8) {
    check(http.get(`${CATALOG}/api/v1/products?q=${pick(SEARCH_TERMS)}`, { tags: { name: 'search' } }), {
      '200': (res) => res.status === 200,
    });
  } else {
    check(http.get(`${CATALOG}/api/v1/products/facets?category=${pick(CATEGORIES)}`, { tags: { name: 'facets' } }), {
      '200': (res) => res.status === 200,
    });
  }
}
