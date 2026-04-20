import { getCurrentLocale, type Locale } from '@/store/localeStore'

export type CurrencyCode = 'BRL' | 'USD'

export function formatCurrency(
  amount: number,
  currency: CurrencyCode | string = 'BRL',
  locale?: Locale
): string {
  const loc = locale ?? getCurrentLocale()
  return new Intl.NumberFormat(loc, {
    style: 'currency',
    currency,
    currencyDisplay: 'symbol',
  }).format(amount)
}

export function formatNumber(
  value: number,
  locale?: Locale,
  options?: Intl.NumberFormatOptions
): string {
  const loc = locale ?? getCurrentLocale()
  return new Intl.NumberFormat(loc, options).format(value)
}

export function formatDate(
  date: Date | string | number,
  locale?: Locale,
  options: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'short', day: 'numeric' }
): string {
  const loc = locale ?? getCurrentLocale()
  const d = date instanceof Date ? date : new Date(date)
  return new Intl.DateTimeFormat(loc, options).format(d)
}

export function formatDateTime(date: Date | string | number, locale?: Locale): string {
  return formatDate(date, locale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatShortDate(date: Date | string | number, locale?: Locale): string {
  return formatDate(date, locale, { month: 'short', day: 'numeric' })
}

export function formatRelativeTime(date: Date | string | number, locale?: Locale): string {
  const loc = locale ?? getCurrentLocale()
  const d = date instanceof Date ? date : new Date(date)
  const diffSec = Math.round((d.getTime() - Date.now()) / 1000)
  const rtf = new Intl.RelativeTimeFormat(loc, { numeric: 'auto' })

  const abs = Math.abs(diffSec)
  if (abs < 60) return rtf.format(diffSec, 'second')
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 2592000) return rtf.format(Math.round(diffSec / 86400), 'day')
  if (abs < 31536000) return rtf.format(Math.round(diffSec / 2592000), 'month')
  return rtf.format(Math.round(diffSec / 31536000), 'year')
}

export function formatCEP(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 8)
  if (digits.length <= 5) return digits
  return `${digits.slice(0, 5)}-${digits.slice(5)}`
}

export function formatCPF(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 11)
  if (digits.length <= 3) return digits
  if (digits.length <= 6) return `${digits.slice(0, 3)}.${digits.slice(3)}`
  if (digits.length <= 9) return `${digits.slice(0, 3)}.${digits.slice(3, 6)}.${digits.slice(6)}`
  return `${digits.slice(0, 3)}.${digits.slice(3, 6)}.${digits.slice(6, 9)}-${digits.slice(9)}`
}

export function formatCNPJ(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 14)
  if (digits.length <= 2) return digits
  if (digits.length <= 5) return `${digits.slice(0, 2)}.${digits.slice(2)}`
  if (digits.length <= 8) return `${digits.slice(0, 2)}.${digits.slice(2, 5)}.${digits.slice(5)}`
  if (digits.length <= 12)
    return `${digits.slice(0, 2)}.${digits.slice(2, 5)}.${digits.slice(5, 8)}/${digits.slice(8)}`
  return `${digits.slice(0, 2)}.${digits.slice(2, 5)}.${digits.slice(5, 8)}/${digits.slice(8, 12)}-${digits.slice(12)}`
}

export function formatPhone(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 11)
  if (digits.length <= 2) return `(${digits}`
  if (digits.length <= 6) return `(${digits.slice(0, 2)}) ${digits.slice(2)}`
  if (digits.length <= 10)
    return `(${digits.slice(0, 2)}) ${digits.slice(2, 6)}-${digits.slice(6)}`
  return `(${digits.slice(0, 2)}) ${digits.slice(2, 7)}-${digits.slice(7)}`
}

export function productCurrency(product: unknown): CurrencyCode {
  const raw =
    product && typeof product === 'object' && 'currency' in product
      ? (product as { currency?: unknown }).currency
      : undefined
  const c = typeof raw === 'string' ? raw.toUpperCase() : undefined
  return c === 'USD' ? 'USD' : 'BRL'
}
