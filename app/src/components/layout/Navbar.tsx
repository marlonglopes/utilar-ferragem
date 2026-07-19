import { useState, useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Search, ShoppingCart, Menu, X, Heart, LogOut, User, Package } from 'lucide-react'
import { LocaleSwitcher } from '@/components/common/LocaleSwitcher'
import { CartDrawer } from '@/components/cart/CartDrawer'
import { AccountMenu } from './AccountMenu'
import { useCartStore } from '@/store/cartStore'
import { useAuthStore } from '@/store/authStore'
import { useFavoritesStore } from '@/store/favoritesStore'
import { useLogout } from '@/hooks/useLogout'
import { cn } from '@/lib/cn'

export function Navbar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [searchExpanded, setSearchExpanded] = useState(false)
  const [cartOpen, setCartOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const cartCount = useCartStore((s) => s.items.reduce((sum, i) => sum + i.quantity, 0))
  const favoritesCount = useFavoritesStore((s) => s.items.length)
  const user = useAuthStore((s) => s.user)
  const doLogout = useLogout()

  function handleSearch(e: React.FormEvent) {
    e.preventDefault()
    const q = inputRef.current?.value.trim()
    if (!q) return
    navigate(`/busca?q=${encodeURIComponent(q)}`)
    setSearchExpanded(false)
  }

  return (
    <header className="bg-brand-orange shadow-sm sticky top-0 z-40">
      <div className="container">
        <div className="flex items-center gap-3 h-14">
          <Link to="/" className="flex items-center gap-2 flex-shrink-0" aria-label={t('brand')}>
            <span className="w-8 h-8 rounded-md bg-brand-blue flex items-center justify-center text-white font-display font-black text-lg italic select-none">
              U
            </span>
            <div className="hidden sm:flex flex-col leading-none">
              <span className="font-display font-black text-lg tracking-tight">
                <span className="text-white">Uti</span>
                <span className="text-brand-blue">Lar</span>
              </span>
              <span className="text-[9px] font-bold tracking-widest uppercase text-white/70">
                {t('tagline')}
              </span>
            </div>
          </Link>

          <form
            onSubmit={handleSearch}
            className={cn(
              'flex-1 relative transition-all duration-200',
              searchExpanded ? 'block' : 'hidden sm:block'
            )}
          >
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400 pointer-events-none"
              aria-hidden
            />
            <input
              ref={inputRef}
              type="search"
              placeholder={t('searchPlaceholder')}
              className="w-full pl-9 pr-4 py-2 rounded-lg bg-white text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none focus:ring-2 focus:ring-brand-blue"
              aria-label={t('search')}
            />
          </form>

          <div className="flex items-center gap-1 ml-auto sm:ml-0">
            <button
              className="sm:hidden flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              onClick={() => setSearchExpanded((v) => !v)}
              aria-label={t('search')}
            >
              {searchExpanded ? <X className="h-5 w-5" /> : <Search className="h-5 w-5" />}
            </button>

            {/*
              Contador de favoritos no cabeçalho: a lista só tem valor se o
              cliente lembrar que ela existe. Sem o número visível, o coração
              vira um botão que não parece levar a lugar nenhum.
            */}
            <Link
              to="/favoritos"
              className="relative flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              aria-label={
                favoritesCount > 0
                  ? `${t('favorites.nav')} (${favoritesCount})`
                  : t('favorites.nav')
              }
            >
              <Heart className="h-5 w-5" aria-hidden />
              {favoritesCount > 0 && (
                <span className="absolute -top-1 -right-1 h-4 min-w-4 px-0.5 rounded-full bg-brand-blue text-white text-[10px] font-bold flex items-center justify-center leading-none">
                  {favoritesCount > 99 ? '99+' : favoritesCount}
                </span>
              )}
            </Link>

            <AccountMenu />

            <button
              onClick={() => setCartOpen(true)}
              className="relative flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              aria-label={t('cart')}
            >
              <ShoppingCart className="h-5 w-5" aria-hidden />
              {cartCount > 0 && (
                <span className="absolute -top-1 -right-1 h-4 min-w-4 px-0.5 rounded-full bg-brand-blue text-white text-[10px] font-bold flex items-center justify-center leading-none">
                  {cartCount > 99 ? '99+' : cartCount}
                </span>
              )}
            </button>

            <LocaleSwitcher className="hidden sm:flex" />

            <button
              className="sm:hidden flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              onClick={() => setMobileOpen((v) => !v)}
              aria-label={t('menu')}
              aria-expanded={mobileOpen}
            >
              {mobileOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
            </button>
          </div>
        </div>
      </div>

      {/*
        Menu mobile. A maioria dos clientes compra no celular, então tudo que
        existe no desktop precisa existir aqui — inclusive "Sair da conta", que
        antes não tinha nenhum caminho em telas pequenas.
      */}
      {mobileOpen && (
        <div className="sm:hidden bg-brand-orange-dark border-t border-white/10 px-4 py-3 flex flex-col gap-1">
          {user ? (
            <>
              <p className="px-1 pb-2 text-xs text-white/70 truncate">
                {t('nav.greeting', { name: user.name.split(' ')[0] })}
              </p>
              <Link
                to="/conta"
                onClick={() => setMobileOpen(false)}
                className="flex items-center gap-2.5 rounded-lg px-1 py-2 text-sm font-medium text-white hover:bg-white/10"
              >
                <User className="h-4 w-4" aria-hidden />
                {t('nav.myAccount')}
              </Link>
              <Link
                to="/conta?tab=pedidos"
                onClick={() => setMobileOpen(false)}
                className="flex items-center gap-2.5 rounded-lg px-1 py-2 text-sm font-medium text-white hover:bg-white/10"
              >
                <Package className="h-4 w-4" aria-hidden />
                {t('nav.myOrders')}
              </Link>
              <Link
                to="/favoritos"
                onClick={() => setMobileOpen(false)}
                className="flex items-center gap-2.5 rounded-lg px-1 py-2 text-sm font-medium text-white hover:bg-white/10"
              >
                <Heart className="h-4 w-4" aria-hidden />
                {t('nav.favorites')}
                {favoritesCount > 0 && (
                  <span className="ml-auto text-xs text-white/70">{favoritesCount}</span>
                )}
              </Link>
              <button
                type="button"
                onClick={() => {
                  setMobileOpen(false)
                  doLogout()
                }}
                className="mt-1 flex items-center gap-2.5 rounded-lg border-t border-white/10 px-1 py-2 pt-3 text-sm font-semibold text-white hover:bg-white/10"
              >
                <LogOut className="h-4 w-4" aria-hidden />
                {t('nav.logout')}
              </button>
            </>
          ) : (
            <>
              <Link
                to="/entrar"
                onClick={() => setMobileOpen(false)}
                className="flex items-center gap-2.5 rounded-lg px-1 py-2 text-sm font-medium text-white hover:bg-white/10"
              >
                <User className="h-4 w-4" aria-hidden />
                {t('nav.login')}
              </Link>
              <Link
                to="/favoritos"
                onClick={() => setMobileOpen(false)}
                className="flex items-center gap-2.5 rounded-lg px-1 py-2 text-sm font-medium text-white hover:bg-white/10"
              >
                <Heart className="h-4 w-4" aria-hidden />
                {t('nav.favorites')}
              </Link>
            </>
          )}
          <div className="mt-2 border-t border-white/10 pt-2">
            <LocaleSwitcher />
          </div>
        </div>
      )}

      <CartDrawer open={cartOpen} onClose={() => setCartOpen(false)} />
    </header>
  )
}
