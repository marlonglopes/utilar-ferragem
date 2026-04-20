import { Link } from 'react-router-dom'
import { cn } from '@/lib/cn'
import { ChevronRight } from 'lucide-react'

export interface BreadcrumbItem {
  label: string
  href?: string
}

export interface BreadcrumbProps {
  items: BreadcrumbItem[]
  className?: string
}

function Breadcrumb({ items, className }: BreadcrumbProps) {
  return (
    <nav aria-label="Breadcrumb" className={cn('flex items-center gap-1 text-sm', className)}>
      <ol className="flex items-center flex-wrap gap-1">
        {items.map((item, i) => {
          const isLast = i === items.length - 1
          return (
            <li key={i} className="flex items-center gap-1">
              {i > 0 && <ChevronRight className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" aria-hidden />}
              {isLast || !item.href ? (
                <span
                  className={cn(
                    isLast ? 'text-gray-900 font-medium' : 'text-gray-500'
                  )}
                  aria-current={isLast ? 'page' : undefined}
                >
                  {item.label}
                </span>
              ) : (
                <Link
                  to={item.href}
                  className="text-brand-blue hover:text-brand-blue-dark hover:underline"
                >
                  {item.label}
                </Link>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

export { Breadcrumb }
