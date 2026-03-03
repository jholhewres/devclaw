import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

interface TooltipProps {
  children: ReactNode
  label: string
  position?: 'top' | 'bottom' | 'left' | 'right'
  className?: string
}

const positions = {
  top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
  bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
  left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  right: 'left-full top-1/2 -translate-y-1/2 ml-2',
}

const arrows = {
  top: 'top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-[#1c1f3a]',
  bottom: 'bottom-full left-1/2 -translate-x-1/2 border-4 border-transparent border-b-[#1c1f3a]',
  left: 'left-full top-1/2 -translate-y-1/2 border-4 border-transparent border-l-[#1c1f3a]',
  right: 'right-full top-1/2 -translate-y-1/2 border-4 border-transparent border-r-[#1c1f3a]',
}

export function Tooltip({ children, label, position = 'right', className }: TooltipProps) {
  const [visible, setVisible] = useState(false)

  return (
    <div
      className={cn('relative inline-flex', className)}
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
    >
      {children}
      {visible && (
        <div className={cn('absolute z-50 pointer-events-none', positions[position])}>
          <div className="relative bg-[#1c1f3a] text-[#f1f5f9] text-xs font-medium px-3 py-1.5 rounded-lg whitespace-nowrap shadow-lg animate-fade-in border border-[rgba(99,102,241,0.15)]">
            {label}
            <div className={cn('absolute', arrows[position])} />
          </div>
        </div>
      )}
    </div>
  )
}
