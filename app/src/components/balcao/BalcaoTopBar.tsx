import { Link, useLocation } from 'react-router-dom'
import { ClipboardCheck, Store, User as UserIcon } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { useBalcaoStore, type BalcaoLevel } from '@/store/balcaoStore'

const LEVEL_LABEL: Record<BalcaoLevel, string> = {
  operator: 'Vendedor',
  supervisor: 'Supervisor',
  manager: 'Gerente',
}

/**
 * Barra superior fixa do PDV: marca, badge "Balcão" e identificação do
 * vendedor/loja. Altura generosa (h-16) porque no tablet ela também é a área de
 * toque para trocar de comanda pelo menu do sistema.
 *
 * Cargo e loja vêm do vínculo (`GET /api/v1/store/me`), não de constante no
 * front: "Loja Centro" fixo no código mentia em toda filial que não fosse a
 * primeira.
 */
export function BalcaoTopBar() {
  const user = useAuthStore((s) => s.user)
  const operator = useBalcaoStore((s) => s.operator)
  const location = useLocation()

  const onApprovals = location.pathname.startsWith('/balcao/aprovacoes')

  return (
    <header className="sticky top-0 z-30 bg-brand-blue text-white shadow-md">
      <div className="flex h-16 items-center gap-3 px-4">
        <div className="flex items-center gap-2">
          <Store className="h-6 w-6 shrink-0" aria-hidden="true" />
          <span className="font-display text-lg font-bold tracking-tight">UtiLar</span>
        </div>

        <span className="rounded-full bg-brand-orange px-3 py-1 text-xs font-bold uppercase tracking-wide">
          Balcão
        </span>

        {/* Só quem homologa vê a fila — o backend recusaria os cliques dos demais. */}
        {operator.canApproveDiscount && !onApprovals && (
          <Link
            to="/balcao/aprovacoes"
            className="ml-2 flex h-12 items-center gap-2 rounded-lg px-3 text-sm font-semibold text-white/90 hover:bg-white/10"
          >
            <ClipboardCheck className="h-5 w-5" aria-hidden="true" />
            <span className="hidden sm:inline">Aprovações</span>
          </Link>
        )}

        <div className="ml-auto flex items-center gap-2 text-right">
          <div className="hidden leading-tight sm:block">
            <p className="text-sm font-semibold">{operator.name || user?.name || 'Vendedor'}</p>
            <p className="text-xs text-white/70">
              {LEVEL_LABEL[operator.level] ?? 'Vendedor'}
              {operator.storeName ? ` · ${operator.storeName}` : ''}
            </p>
          </div>
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-white/15">
            <UserIcon className="h-5 w-5" aria-hidden="true" />
          </span>
        </div>
      </div>
    </header>
  )
}
