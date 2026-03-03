import { cn } from '@/lib/utils'

interface AvatarProps {
  name?: string
  icon?: React.ElementType
  src?: string
  size?: 'sm' | 'md' | 'lg'
  status?: 'online' | 'offline' | 'busy' | 'away'
  className?: string
}

const sizes = {
  sm: 'h-8 w-8 text-xs',
  md: 'h-10 w-10 text-sm',
  lg: 'h-12 w-12 text-base',
}

const iconSizes = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-6 w-6',
}

const statusColors = {
  online: 'bg-[#10b981]',
  offline: 'bg-[#64748b]',
  busy: 'bg-[#f43f5e]',
  away: 'bg-[#f59e0b]',
}

const statusDot = {
  sm: 'h-2 w-2 -right-0.5 -bottom-0.5',
  md: 'h-2.5 w-2.5 -right-0.5 -bottom-0.5',
  lg: 'h-3 w-3 -right-0.5 -bottom-0.5',
}

export function Avatar({ name, icon: Icon, src, size = 'md', status, className }: AvatarProps) {
  const initials = name?.split(' ').map(w => w[0]).join('').slice(0, 2).toUpperCase()

  return (
    <div className="relative inline-flex shrink-0">
      <div
        className={cn(
          'flex items-center justify-center rounded-full bg-gradient-brand font-medium text-white',
          sizes[size],
          className,
        )}
      >
        {src ? (
          <img src={src} alt={name || ''} className="h-full w-full rounded-full object-cover" />
        ) : Icon ? (
          <Icon className={cn('text-white', iconSizes[size])} />
        ) : (
          initials
        )}
      </div>
      {status && (
        <span
          className={cn(
            'absolute rounded-full ring-2 ring-[#0b0d17]',
            statusColors[status],
            statusDot[size],
            status === 'online' && 'animate-pulse-glow',
          )}
        />
      )}
    </div>
  )
}
