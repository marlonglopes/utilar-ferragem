import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/cn'

interface SpecSheetProps {
  specs?: Record<string, string>
  className?: string
}

export function SpecSheet({ specs, className }: SpecSheetProps) {
  const { t } = useTranslation('catalog')

  if (!specs || Object.keys(specs).length === 0) {
    return <p className="text-sm text-gray-500">{t('product.noSpecs')}</p>
  }

  return (
    <table className={cn('w-full text-sm border-collapse', className)}>
      <tbody>
        {Object.entries(specs).map(([key, value], i) => (
          <tr key={key} className={i % 2 === 0 ? 'bg-gray-50' : 'bg-white'}>
            <td className="py-2 px-3 font-medium text-gray-700 w-2/5 rounded-l">{key}</td>
            <td className="py-2 px-3 text-gray-900 rounded-r">{value}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
