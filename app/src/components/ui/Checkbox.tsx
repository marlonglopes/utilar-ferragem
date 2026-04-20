import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: string
  hint?: string
  error?: string
}

const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, hint, error, className, id, ...props }, ref) => {
    const checkId = id ?? label?.toLowerCase().replace(/\s+/g, '-')
    return (
      <div className="flex flex-col gap-0.5">
        <label htmlFor={checkId} className="inline-flex items-center gap-2 cursor-pointer">
          <input
            ref={ref}
            type="checkbox"
            id={checkId}
            className={cn(
              'h-4 w-4 rounded border-gray-300 text-brand-orange',
              'focus:ring-2 focus:ring-brand-orange focus:ring-offset-0',
              'disabled:cursor-not-allowed disabled:opacity-50',
              error && 'border-red-500',
              className
            )}
            {...props}
          />
          {label && <span className="text-sm text-gray-700">{label}</span>}
        </label>
        {hint && !error && <p className="ml-6 text-xs text-gray-500">{hint}</p>}
        {error && <p className="ml-6 text-xs text-red-600">{error}</p>}
      </div>
    )
  }
)
Checkbox.displayName = 'Checkbox'

export { Checkbox }
