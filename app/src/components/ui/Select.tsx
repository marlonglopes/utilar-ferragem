import { forwardRef, type SelectHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'
import { ChevronDown } from 'lucide-react'

export interface SelectOption {
  value: string
  label: string
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string
  error?: string
  hint?: string
  options: SelectOption[]
  placeholder?: string
}

const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ label, error, hint, options, placeholder, className, id, ...props }, ref) => {
    const selectId = id ?? label?.toLowerCase().replace(/\s+/g, '-')
    return (
      <div className="flex flex-col gap-1">
        {label && (
          <label htmlFor={selectId} className="text-sm font-medium text-gray-700">
            {label}
          </label>
        )}
        <div className="relative">
          <select
            ref={ref}
            id={selectId}
            className={cn(
              'w-full appearance-none rounded-lg border border-gray-300 bg-white px-3 py-2 pr-9 text-sm text-gray-900',
              'focus:outline-none focus:ring-2 focus:ring-brand-orange focus:border-transparent',
              'disabled:bg-gray-50 disabled:text-gray-500 disabled:cursor-not-allowed',
              error && 'border-red-500 focus:ring-red-500',
              className
            )}
            {...props}
          >
            {placeholder && (
              <option value="" disabled>
                {placeholder}
              </option>
            )}
            {options.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <ChevronDown
            className="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400 pointer-events-none"
            aria-hidden
          />
        </div>
        {error && <p className="text-xs text-red-600">{error}</p>}
        {hint && !error && <p className="text-xs text-gray-500">{hint}</p>}
      </div>
    )
  }
)
Select.displayName = 'Select'

export { Select }
