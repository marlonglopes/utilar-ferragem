import { forwardRef, type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export interface RadioProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: string
  hint?: string
}

const Radio = forwardRef<HTMLInputElement, RadioProps>(
  ({ label, hint, className, id, ...props }, ref) => {
    const radioId = id ?? label?.toLowerCase().replace(/\s+/g, '-')
    return (
      <div className="flex flex-col gap-0.5">
        <label htmlFor={radioId} className="inline-flex items-center gap-2 cursor-pointer">
          <input
            ref={ref}
            type="radio"
            id={radioId}
            className={cn(
              'h-4 w-4 border-gray-300 text-brand-orange',
              'focus:ring-2 focus:ring-brand-orange focus:ring-offset-0',
              'disabled:cursor-not-allowed disabled:opacity-50',
              className
            )}
            {...props}
          />
          {label && <span className="text-sm text-gray-700">{label}</span>}
        </label>
        {hint && <p className="ml-6 text-xs text-gray-500">{hint}</p>}
      </div>
    )
  }
)
Radio.displayName = 'Radio'

export { Radio }
