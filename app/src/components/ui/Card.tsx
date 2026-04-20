import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  padding?: 'none' | 'sm' | 'md' | 'lg'
  shadow?: 'none' | 'sm' | 'md'
  border?: boolean
}

const paddings = {
  none: '',
  sm: 'p-3',
  md: 'p-4',
  lg: 'p-6',
}

const shadows = {
  none: '',
  sm: 'shadow-sm',
  md: 'shadow-md',
}

function Card({ padding = 'md', shadow = 'sm', border = true, className, children, ...props }: CardProps) {
  return (
    <div
      className={cn(
        'rounded-xl bg-white',
        paddings[padding],
        shadows[shadow],
        border && 'border border-gray-200',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

function CardHeader({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('mb-4', className)} {...props} />
}

function CardTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn('text-base font-semibold text-gray-900', className)} {...props} />
}

function CardBody({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('', className)} {...props} />
}

function CardFooter({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('mt-4 pt-4 border-t border-gray-100 flex items-center gap-2', className)} {...props} />
  )
}

export { Card, CardHeader, CardTitle, CardBody, CardFooter }
