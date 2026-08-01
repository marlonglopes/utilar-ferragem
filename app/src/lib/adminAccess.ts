/**
 * Matriz de acesso do painel por PERSONA — fonte única do front.
 *
 * ⚠️ Isto é CONFORTO, não segurança. A fronteira de verdade é o 403 de cada
 * serviço (RequireRole no backend). Aqui só escondemos do menu o que a pessoa
 * não pode abrir, para ela não bater numa tela que vai falhar. Se esta matriz e
 * o backend divergirem, quem manda é o backend. Mantê-los alinhados:
 *   - /admin (Visão geral): order /admin/overview → admin only (tem margem/custo)
 *   - /admin/contabil, /admin/trilha: payment /ledger/* → admin + contador
 *   - /admin/observabilidade: catalog /observability → admin + contador
 *   - /admin/pedidos: order /admin/orders → admin, contador(leitura), vendas, almoxarife
 *   - /admin/atividade, /produtos, /categorias, /importar: catalog /admin/* (tem
 *     CUSTO) → admin + vendas
 *   - /admin/operadores: auth /admin/operators → admin only
 *   - /admin/vendedores: order /admin/sellers → admin only
 */

export type StaffRole = 'admin' | 'contador' | 'vendas' | 'almoxarife'

export const STAFF_ROLES: readonly StaffRole[] = ['admin', 'contador', 'vendas', 'almoxarife']

export function isStaffRole(r?: string | null): r is StaffRole {
  return !!r && (STAFF_ROLES as readonly string[]).includes(r)
}

/** Seções na ORDEM do menu; `roles` = quem pode abrir (espelha o 403 do backend). */
export const ADMIN_SECTIONS: ReadonlyArray<{ path: string; roles: readonly StaffRole[] }> = [
  { path: '/admin', roles: ['admin'] }, // Visão geral: faturamento/margem
  { path: '/admin/contabil', roles: ['admin', 'contador'] },
  { path: '/admin/pedidos', roles: ['admin', 'contador', 'vendas', 'almoxarife'] },
  { path: '/admin/operadores', roles: ['admin'] },
  { path: '/admin/vendedores', roles: ['admin'] },
  { path: '/admin/produtos', roles: ['admin', 'vendas'] }, // tem custo
  { path: '/admin/categorias', roles: ['admin', 'vendas'] },
  { path: '/admin/importar', roles: ['admin', 'vendas'] },
  { path: '/admin/atividade', roles: ['admin', 'vendas'] }, // trilha do catálogo (custo)
  { path: '/admin/trilha', roles: ['admin', 'contador'] }, // trilha contábil (sem custo)
  { path: '/admin/observabilidade', roles: ['admin', 'contador'] },
]

/** A seção mais específica que casa com o caminho (ex.: /admin/produtos/novo → /admin/produtos). */
function sectionFor(path: string) {
  return [...ADMIN_SECTIONS]
    .sort((a, b) => b.path.length - a.path.length)
    .find((s) => path === s.path || path.startsWith(s.path + '/'))
}

/** A pessoa (papel) pode abrir esta rota do painel? Fail-closed: papel não-staff → não. */
export function canAccessAdmin(path: string, role?: string | null): boolean {
  if (!isStaffRole(role)) return false
  const s = sectionFor(path)
  if (!s) return role === 'admin' // rota de admin não mapeada → só admin
  return (s.roles as readonly string[]).includes(role)
}

/** Primeira seção (na ordem do menu) que o papel consegue abrir — o "home" dele. */
export function landingFor(role: StaffRole): string {
  const first = ADMIN_SECTIONS.find((s) => (s.roles as readonly string[]).includes(role))
  return first ? first.path : '/admin'
}
