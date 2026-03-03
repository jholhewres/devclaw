import { cn } from '@/lib/utils'

interface SkeletonProps {
  className?: string
  variant?: 'text' | 'circular' | 'rectangular'
  width?: string | number
  height?: string | number
}

export function Skeleton({ className, variant = 'rectangular', width, height }: SkeletonProps) {
  return (
    <div
      className={cn(
        'skeleton-shimmer',
        variant === 'circular' && 'rounded-full',
        variant === 'text' && 'rounded-md h-4',
        variant === 'rectangular' && 'rounded-xl',
        className,
      )}
      style={{ width, height }}
    />
  )
}

/** Pre-built skeleton for a metric card */
export function SkeletonCard({ className }: { className?: string }) {
  return (
    <div className={cn('rounded-2xl border border-[rgba(99,102,241,0.08)] bg-[#14172b] p-5', className)}>
      <Skeleton variant="text" className="w-20 h-3 mb-3" />
      <Skeleton variant="text" className="w-16 h-7" />
    </div>
  )
}

/** Pre-built skeleton for a list item */
export function SkeletonListItem({ className }: { className?: string }) {
  return (
    <div className={cn('flex items-center gap-3 rounded-xl border border-[rgba(99,102,241,0.08)] bg-[#14172b] px-4 py-3.5', className)}>
      <Skeleton variant="circular" className="h-8 w-8" />
      <div className="flex-1 space-y-2">
        <Skeleton variant="text" className="w-32" />
        <Skeleton variant="text" className="w-20 h-3" />
      </div>
    </div>
  )
}
