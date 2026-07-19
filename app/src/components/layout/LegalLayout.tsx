import type { ReactNode } from 'react'
import { Breadcrumb } from '@/components/ui'

interface LegalLayoutProps {
  title: string
  /** Subtítulo curto abaixo do H1 (ex: resumo do documento). */
  subtitle?: string
  /** Data da última atualização exibida no cabeçalho. */
  updatedAt?: string
  /** Aviso de revisão jurídica pendente — some quando o texto for aprovado. */
  reviewNotice?: boolean
  children: ReactNode
}

/**
 * Casca das páginas institucionais e legais: breadcrumb, cabeçalho e um
 * container de texto com largura de leitura confortável (~70 caracteres).
 */
export function LegalLayout({
  title,
  subtitle,
  updatedAt,
  reviewNotice = false,
  children,
}: LegalLayoutProps) {
  return (
    <div className="container py-8">
      <Breadcrumb items={[{ label: 'Início', href: '/' }, { label: title }]} className="mb-6" />

      <header className="mb-8 max-w-3xl">
        <h1 className="font-display font-black text-3xl text-gray-900 leading-tight">{title}</h1>
        {subtitle && <p className="mt-3 text-gray-600 leading-relaxed">{subtitle}</p>}
        {updatedAt && (
          <p className="mt-3 text-sm text-gray-400">Última atualização: {updatedAt}</p>
        )}
      </header>

      {reviewNotice && (
        <div className="mb-8 max-w-3xl rounded-xl border-l-4 border-brand-gold bg-brand-gold/10 p-4">
          <p className="text-sm text-gray-800">
            <strong className="font-semibold">Documento em revisão.</strong> Este texto é uma minuta
            e ainda passará por validação jurídica. Os campos entre colchetes serão preenchidos com
            os dados cadastrais da empresa antes da publicação definitiva.
          </p>
        </div>
      )}

      <div className="max-w-3xl text-[15px] leading-relaxed text-gray-700 [&_h2]:font-display [&_h2]:font-bold [&_h2]:text-xl [&_h2]:text-gray-900 [&_h2]:mt-10 [&_h2]:mb-3 [&_h3]:font-semibold [&_h3]:text-base [&_h3]:text-gray-900 [&_h3]:mt-6 [&_h3]:mb-2 [&_p]:mb-4 [&_ul]:mb-4 [&_ul]:list-disc [&_ul]:pl-6 [&_ul]:space-y-1.5 [&_ol]:mb-4 [&_ol]:list-decimal [&_ol]:pl-6 [&_ol]:space-y-1.5 [&_a]:text-brand-orange [&_a]:underline [&_strong]:text-gray-900">
        {children}
      </div>
    </div>
  )
}
