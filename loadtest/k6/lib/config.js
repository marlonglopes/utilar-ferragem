// Config compartilhada dos testes de carga k6 do Utilar.
//
// Em DEV local os 4 serviços ficam em portas separadas (8090-8093). Em produção
// ficam todos atrás de um host só (api.utilarferragem.com.br) — passe BASE_URL
// pra apontar tudo pra lá.

const BASE = __ENV.BASE_URL || '';

export const CATALOG = __ENV.CATALOG_URL || BASE || 'http://localhost:8091';
export const AUTH = __ENV.AUTH_URL || BASE || 'http://localhost:8093';
export const ORDER = __ENV.ORDER_URL || BASE || 'http://localhost:8092';
export const PAYMENT = __ENV.PAYMENT_URL || BASE || 'http://localhost:8090';

// Usuário de teste do seed (senha universal).
export const TEST_USER = {
  email: __ENV.TEST_EMAIL || 'test1@utilar.com.br',
  password: __ENV.TEST_PASSWORD || 'utilar123',
};

// Categorias reais do seed pra variar a navegação.
export const CATEGORIES = ['ferramentas', 'construcao', 'eletrica', 'fixacao', 'hidraulica'];
export const SEARCH_TERMS = ['furadeira', 'cimento', 'parafuso', 'cabo', 'tinta', 'broca'];

export function pick(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

// login devolve o accessToken (ou '' se falhar) — usado nos fluxos autenticados.
import http from 'k6/http';
export function login() {
  const res = http.post(
    `${AUTH}/api/v1/auth/login`,
    JSON.stringify(TEST_USER),
    { headers: { 'Content-Type': 'application/json' }, tags: { name: 'auth/login' } }
  );
  try {
    return res.json('accessToken') || '';
  } catch (_) {
    return '';
  }
}
