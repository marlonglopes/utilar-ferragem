import { Store, User as UserIcon } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { useBalcaoStore } from '@/store/balcaoStore'

const ROLE_LABEL: Record<string, string> = {
  operator: 'Vendedor',
  supervisor: 'Supervisor',
  manager: 'Gerente',
}

/**
 * Barra superior fixa do PDV: marca, badge "Balcão" e identificação do
 * vendedor/loja. Altura generosa (h-16) porque no tablet ela também é a área de
 * toque para trocar de comanda pelo menu do sistema.
 */
export function BalcaoTopBar() {
  const user = useAuthStore((s) => s.user)
  const role = useBalcaoStore((s) => s.role)

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

        <div className="ml-auto flex items-center gap-2 text-right">
          <div className="hidden sm:block leading-tight">
            <p className="text-sm font-semibold">{user?.name ?? 'Vendedor'}</p>
            <p className="text-xs text-white/70">
              {ROLE_LABEL[role] ?? 'Vendedor'} · Loja Centro
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
