import { useState, useRef, useEffect, type KeyboardEvent } from 'react'
import { ArrowUp, Square } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatInputProps {
  onSend: (message: string) => void
  onAbort?: () => void
  isStreaming?: boolean
  disabled?: boolean
  placeholder?: string
}

export function ChatInput({
  onSend, onAbort, isStreaming = false, disabled = false, placeholder = 'Escreva sua mensagem...',
}: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [value])

  useEffect(() => { textareaRef.current?.focus() }, [])

  const handleSend = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    if (textareaRef.current) textareaRef.current.style.height = 'auto'
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (isStreaming) return
      handleSend()
    }
  }

  return (
    <div className="border-t border-white/[0.06] bg-[var(--color-dc-darker)] px-6 py-5">
      <div
        className={cn(
          'flex items-end gap-4 rounded-2xl border border-white/[0.08] bg-[var(--color-dc-dark)] px-5 py-4',
          'transition-all',
          'focus-within:border-orange-500/30 focus-within:ring-2 focus-within:ring-orange-500/10',
        )}
      >
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          disabled={disabled}
          rows={1}
          className="flex-1 resize-none bg-transparent text-base text-white outline-none placeholder:text-gray-600 max-h-[200px] disabled:opacity-50"
        />
        {isStreaming ? (
          <button
            onClick={() => onAbort?.()}
            className="flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center rounded-xl bg-red-500/15 text-red-400 ring-1 ring-red-500/20 transition-all hover:bg-red-500/25"
            title="Parar"
          >
            <Square className="h-4.5 w-4.5" fill="currentColor" />
          </button>
        ) : (
          <button
            onClick={handleSend}
            disabled={!value.trim() || disabled}
            className={cn(
              'flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center rounded-xl transition-all',
              value.trim()
                ? 'bg-gradient-to-r from-orange-500 to-amber-500 text-white shadow-lg shadow-orange-500/20 hover:shadow-orange-500/30'
                : 'bg-white/[0.06] text-gray-600',
            )}
            title="Enviar"
          >
            <ArrowUp className="h-5 w-5" />
          </button>
        )}
      </div>
      <p className="mt-3 text-center text-xs text-gray-700">
        DevClaw pode cometer erros. Considere verificar informacoes importantes.
      </p>
    </div>
  )
}
