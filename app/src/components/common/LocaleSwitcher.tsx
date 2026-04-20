import { useLocaleStore, SUPPORTED_LOCALES } from '@/store/localeStore'
import { cn } from '@/lib/cn'

const LOCALE_LABELS: Record<string, string> = {
  'pt-BR': 'PT',
  en: 'EN',
}

export interface LocaleSwitcherProps {
  className?: string
}

export function LocaleSwitcher({ className }: LocaleSwitcherProps) {
  const { locale, setLocale } = useLocaleStore()

  return (
    <div className={cn('flex items-center gap-0.5', className)} role="group" aria-label="Idioma">
      {SUPPORTED_LOCALES.map((loc) => (
        <button
          key={loc}
          onClick={() => setLocale(loc)}
          className={cn(
            'px-2 py-1 text-xs font-semibold rounded transition-colors',
            locale === loc
              ? 'bg-brand-orange text-white'
              : 'text-gray-500 hover:text-gray-800 hover:bg-gray-100'
          )}
          aria-pressed={locale === loc}
        >
          {LOCALE_LABELS[loc]}
        </button>
      ))}
    </div>
  )
}
