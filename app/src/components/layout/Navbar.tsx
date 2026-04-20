import { useState, useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Search, User, ShoppingCart, Menu, X } from 'lucide-react'
import { LocaleSwitcher } from '@/components/common/LocaleSwitcher'
import { cn } from '@/lib/cn'

export function Navbar() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [searchExpanded, setSearchExpanded] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

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

            <Link
              to="/conta"
              className="flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              aria-label={t('account')}
            >
              <User className="h-5 w-5" aria-hidden />
            </Link>

            <Link
              to="/carrinho"
              className="flex items-center justify-center h-8 w-8 rounded-lg text-white hover:bg-white/10"
              aria-label={t('cart')}
            >
              <ShoppingCart className="h-5 w-5" aria-hidden />
            </Link>

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

      {mobileOpen && (
        <div className="sm:hidden bg-brand-orange-dark border-t border-white/10 px-4 py-3">
          <LocaleSwitcher />
        </div>
      )}
    </header>
  )
}
