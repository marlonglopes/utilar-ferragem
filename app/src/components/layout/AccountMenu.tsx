import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { User, Package, Heart, LogOut, ChevronDown } from 'lucide-react'
import { useAuthStore } from '@/store/authStore'
import { useLogout } from '@/hooks/useLogout'
import { cn } from '@/lib/cn'

function initialsOf(name: string): string {
  return name
    .split(' ')
    .map((w) => w[0])
    .filter(Boolean)
    .slice(0, 2)
    .join('')
    .toUpperCase()
}

/**
 * Menu da conta no cabeçalho.
 *
 * Antes disso, "Sair" só existia enterrado no topo da página /conta — quem
 * quisesse sair precisava adivinhar que a saída ficava dentro da conta. Em
 * dispositivo compartilhado (o balcão da obra, o celular da família) uma sessão
 * que não dá pra encerrar é um problema de privacidade, não de conveniência.
 */
export function AccountMenu({ className }: { className?: string }) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const doLogout = useLogout()
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)

  // Fecha em clique fora e em Escape. Sem isso o menu fica aberto por cima do
  // catálogo enquanto o cliente navega — em celular é pior, porque cobre o
  // primeiro produto da vitrine.
  useEffect(() => {
    if (!open) return
    function onPointerDown(e: MouseEvent | TouchEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        setOpen(false)
        buttonRef.current?.focus()
      }
    }
    document.addEventListener('mousedown', onPointerDown)
    document.addEventListener('touchstart', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onPointerDown)
      document.removeEventListener('touchstart', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  // Visitante não tem menu: vira link direto pro login, um toque a menos.
  if (!user) {
    return (
      <Link
        to="/entrar"
        className={cn(
          'flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10',
          className
        )}
        aria-label={t('nav.login')}
      >
        <User className="h-5 w-5" aria-hidden />
      </Link>
    )
  }

  const items = [
    { to: '/conta', label: t('nav.myAccount'), icon: User },
    { to: '/conta?tab=pedidos', label: t('nav.myOrders'), icon: Package },
    { to: '/favoritos', label: t('nav.favorites'), icon: Heart },
  ]

  return (
    <div ref={wrapRef} className={cn('relative', className)}>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 h-8 px-1 rounded-lg text-white hover:bg-white/10"
        aria-label={t('nav.accountMenu')}
        aria-expanded={open}
        aria-haspopup="menu"
      >
        <span className="w-6 h-6 rounded-full bg-brand-blue flex items-center justify-center text-[10px] font-bold text-white leading-none">
          {initialsOf(user.name)}
        </span>
        <ChevronDown
          className={cn('h-3.5 w-3.5 transition-transform', open && 'rotate-180')}
          aria-hidden
        />
      </button>

      {open && (
        <div
          role="menu"
          aria-label={t('nav.accountMenu')}
          className="absolute right-0 top-full mt-1.5 w-56 rounded-xl border border-gray-200 bg-white py-1.5 shadow-lg z-50"
        >
          <p className="px-3 pb-1.5 text-xs text-gray-400 truncate border-b border-gray-100 mb-1.5">
            {user.email}
          </p>
          {items.map(({ to, label, icon: Icon }) => (
            <Link
              key={to}
              to={to}
              role="menuitem"
              onClick={() => setOpen(false)}
              className="flex items-center gap-2.5 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              <Icon className="h-4 w-4 text-gray-400" aria-hidden />
              {label}
            </Link>
          ))}
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false)
              doLogout()
            }}
            className="mt-1.5 flex w-full items-center gap-2.5 border-t border-gray-100 px-3 py-2 pt-3 text-sm font-medium text-red-600 hover:bg-red-50"
          >
            <LogOut className="h-4 w-4" aria-hidden />
            {t('nav.logout')}
          </button>
        </div>
      )}
    </div>
  )
}
