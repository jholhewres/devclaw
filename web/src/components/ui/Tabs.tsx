import { cn } from '@/lib/utils'

interface Tab {
  id: string
  label: string
  icon?: React.ElementType
  count?: number
}

interface TabsProps {
  tabs: Tab[]
  activeTab: string
  onChange: (id: string) => void
  className?: string
}

export function Tabs({ tabs, activeTab, onChange, className }: TabsProps) {
  return (
    <div className={cn('flex gap-1 rounded-xl bg-[#14172b] p-1 border border-[rgba(99,102,241,0.1)]', className)}>
      {tabs.map(tab => {
        const active = tab.id === activeTab
        const Icon = tab.icon
        return (
          <button
            key={tab.id}
            onClick={() => onChange(tab.id)}
            className={cn(
              'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-all duration-200',
              active
                ? 'bg-[#6366f1] text-white shadow-sm'
                : 'text-[#64748b] hover:text-[#f1f5f9] hover:bg-white/5',
            )}
          >
            {Icon && <Icon className="h-4 w-4" />}
            {tab.label}
            {tab.count !== undefined && (
              <span className={cn(
                'ml-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold',
                active ? 'bg-white/20' : 'bg-[rgba(99,102,241,0.1)] text-[#818cf8]',
              )}>
                {tab.count}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
