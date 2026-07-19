import { useState, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import {
  Activity,
  BookOpenCheck,
  LayoutDashboard,
  Menu,
  Package,
  ScrollText,
  Upload,
  Users,
  X,
} from 'lucide-react'
import { cn } from '@/lib/cn'
import { isAdminApiEnabled } from '@/lib/adminApi'

/**
 * Chrome do painel administrativo.
 *
 * Fica FORA do `PublicLayout` — como o balcão. O painel tem navegação própria e
 * o dono, no meio de uma conferência contábil, não deve ter um caminho de um
 * toque para a vitrine da loja.
 *
 * Responsivo por decisão explícita: no celular a navegação vira uma barra
 * horizontal rolável, não um menu escondido atrás de hambúrguer. O dono abre o
 * painel no celular para ver a venda do dia — trocar de aba tem que custar um
 * toque, não dois.
 */

const NAV = [
  { to: '/admin', label: 'Visão geral', short: 'Geral', icon: LayoutDashboard, end: true },
  { to: '/admin/contabil', label: 'Auditoria contábil', short: 'Contábil', icon: BookOpenCheck },
  { to: '/admin/vendedores', label: 'Vendedores', short: 'Vendedores', icon: Users },
  { to: '/admin/produtos', label: 'Produtos', short: 'Produtos', icon: Package },
  { to: '/admin/importar', label: 'Importar produtos', short: 'Importar', icon: Upload },
  { to: '/admin/trilha', label: 'Trilha de auditoria', short: 'Trilha', icon: ScrollText },
  { to: '/admin/observabilidade', label: 'Observabilidade', short: 'Saúde', icon: Activity },
]

export function AdminShell({
  title,
  description,
  toolbar,
  children,
}: {
  title: string
  description?: string
  toolbar?: ReactNode
  children: ReactNode
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const location = useLocation()

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="sticky top-0 z-30 border-b border-gray-200 bg-white">
        <div className="flex items-center gap-3 px-3 py-2.5 sm:px-4">
          <span className="rounded bg-brand-blue px-2 py-1 font-display text-xs font-bold uppercase tracking-wider text-white">
            Utilar · Admin
          </span>
          {!isAdminApiEnabled && (
            // Aviso permanente: sem isso alguém apresenta o número de mock numa
            // reunião achando que é faturamento real.
            <span className="rounded bg-amber-50 px-2 py-1 text-[11px] font-semibold text-amber-900 ring-1 ring-inset ring-amber-600/25">
              Dados de demonstração
            </span>
          )}
          <div className="flex-1" />
          <button
            type="button"
            className="rounded p-1.5 text-gray-600 hover:bg-gray-100 lg:hidden"
            onClick={() => setMenuOpen((o) => !o)}
            aria-expanded={menuOpen}
            aria-label={menuOpen ? 'Fechar navegação' : 'Abrir navegação'}
          >
            {menuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>
        </div>

        {/* Navegação: barra rolável no celular, linha completa no desktop. */}
        <nav
          className={cn(
            'border-t border-gray-100 px-2 sm:px-3',
            menuOpen ? 'block' : 'hidden lg:block',
          )}
          aria-label="Seções do painel"
        >
          <ul className="flex gap-1 overflow-x-auto py-1 lg:overflow-visible">
            {NAV.map((item) => {
              const Icon = item.icon
              return (
                <li key={item.to} className="shrink-0">
                  <NavLink
                    to={{ pathname: item.to, search: location.search }}
                    end={item.end}
                    onClick={() => setMenuOpen(false)}
                    className={({ isActive }) =>
                      cn(
                        'flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-semibold transition-colors',
                        isActive
                          ? 'bg-brand-blue-light text-brand-blue'
                          : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900',
                      )
                    }
                  >
                    <Icon className="h-4 w-4" aria-hidden="true" />
                    <span className="lg:hidden">{item.short}</span>
                    <span className="hidden lg:inline">{item.label}</span>
                  </NavLink>
                </li>
              )
            })}
          </ul>
        </nav>
      </header>

      <main className="mx-auto max-w-[1600px] px-3 py-4 sm:px-4">
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div className="min-w-0">
            <h1 className="font-display text-lg font-bold text-gray-900 sm:text-xl">{title}</h1>
            {description && <p className="mt-0.5 text-xs text-gray-500 sm:text-sm">{description}</p>}
          </div>
          {toolbar}
        </div>
        {children}
      </main>
    </div>
  )
}
