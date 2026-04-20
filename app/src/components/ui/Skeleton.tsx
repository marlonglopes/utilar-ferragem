import { type HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  variant?: 'rect' | 'circle' | 'text'
  lines?: number
}

function Skeleton({ variant = 'rect', lines = 1, className, ...props }: SkeletonProps) {
  if (variant === 'text' && lines > 1) {
    return (
      <div className="flex flex-col gap-2">
        {Array.from({ length: lines }).map((_, i) => (
          <div
            key={i}
            className={cn(
              'animate-pulse rounded bg-gray-200',
              i === lines - 1 ? 'w-3/4' : 'w-full',
              'h-4',
              className
            )}
          />
        ))}
      </div>
    )
  }

  return (
    <div
      className={cn(
        'animate-pulse bg-gray-200',
        variant === 'circle' ? 'rounded-full' : 'rounded',
        variant === 'text' && 'h-4 w-full',
        className
      )}
      {...props}
    />
  )
}

export { Skeleton }
