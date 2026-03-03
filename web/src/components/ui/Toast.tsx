import { useEffect, useState, useCallback, createContext, useContext, type ReactNode } from 'react'
import { CheckCircle2, AlertCircle, AlertTriangle, Info, X } from 'lucide-react'
import { cn } from '@/lib/utils'

type ToastType = 'success' | 'error' | 'warning' | 'info'

interface Toast {
  id: string
  type: ToastType
  message: string
  duration?: number
}

interface ToastContextValue {
  toast: (type: ToastType, message: string, duration?: number) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const removeToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  const toast = useCallback((type: ToastType, message: string, duration = 4000) => {
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 6)}`
    setToasts(prev => [...prev, { id, type, message, duration }])
  }, [])

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div className="fixed bottom-6 right-6 z-[100] flex flex-col gap-2">
        {toasts.map(t => (
          <ToastItem key={t.id} toast={t} onDismiss={() => removeToast(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  )
}

const icons = {
  success: CheckCircle2,
  error: AlertCircle,
  warning: AlertTriangle,
  info: Info,
}

const colors = {
  success: { bg: 'bg-[#10b981]/10', border: 'border-[#10b981]/20', text: 'text-[#10b981]' },
  error: { bg: 'bg-[#f43f5e]/10', border: 'border-[#f43f5e]/20', text: 'text-[#f43f5e]' },
  warning: { bg: 'bg-[#f59e0b]/10', border: 'border-[#f59e0b]/20', text: 'text-[#f59e0b]' },
  info: { bg: 'bg-[#6366f1]/10', border: 'border-[#6366f1]/20', text: 'text-[#818cf8]' },
}

function ToastItem({ toast, onDismiss }: { toast: Toast; onDismiss: () => void }) {
  const [exiting, setExiting] = useState(false)
  const Icon = icons[toast.type]
  const color = colors[toast.type]

  useEffect(() => {
    if (!toast.duration) return
    const timer = setTimeout(() => {
      setExiting(true)
      setTimeout(onDismiss, 200)
    }, toast.duration)
    return () => clearTimeout(timer)
  }, [toast.duration, onDismiss])

  return (
    <div
      className={cn(
        'flex items-center gap-3 rounded-xl border px-4 py-3 shadow-lg min-w-[280px] max-w-[400px]',
        'bg-[#14172b] backdrop-blur-md',
        color.border,
        exiting ? 'animate-fade-out opacity-0 translate-y-2' : 'animate-slide-up',
      )}
      style={{ transition: exiting ? 'all 0.2s ease-in' : undefined }}
    >
      <Icon className={cn('h-4 w-4 shrink-0', color.text)} />
      <p className="flex-1 text-sm text-[#f1f5f9]">{toast.message}</p>
      <button
        onClick={() => { setExiting(true); setTimeout(onDismiss, 200) }}
        className="shrink-0 text-[#64748b] hover:text-[#f1f5f9] transition-colors"
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </div>
  )
}
