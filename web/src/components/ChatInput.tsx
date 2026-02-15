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

/**
 * Área de input do chat. Estilo Claude: textarea expansível,
 * botão de enviar/parar, Enter para enviar, Shift+Enter para nova linha.
 */
export function ChatInput({
  onSend,
  onAbort,
  isStreaming = false,
  disabled = false,
  placeholder = 'Escreva sua mensagem...',
}: ChatInputProps) {
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  /* Auto-resize */
  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [value])

  /* Focus ao montar */
  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  const handleSend = () => {
    const trimmed = value.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setValue('')
    // Reset height
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (isStreaming) return
      handleSend()
    }
  }

  return (
    <div className="border-t border-zinc-200 bg-white px-4 py-3 dark:border-zinc-800 dark:bg-zinc-950">
      <div
        className={cn(
          'flex items-end gap-2 rounded-xl border border-zinc-200 bg-zinc-50 px-3 py-2',
          'dark:border-zinc-700 dark:bg-zinc-900',
          'focus-within:border-zinc-400 dark:focus-within:border-zinc-500',
          'transition-colors',
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
          className={cn(
            'flex-1 resize-none bg-transparent text-sm outline-none',
            'placeholder:text-zinc-400',
            'max-h-[200px]',
            'disabled:opacity-50',
          )}
        />

        {isStreaming ? (
          <button
            onClick={() => onAbort?.()}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-zinc-900 text-white transition-colors hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
            title="Parar geração"
          >
            <Square className="h-3.5 w-3.5" fill="currentColor" />
          </button>
        ) : (
          <button
            onClick={handleSend}
            disabled={!value.trim() || disabled}
            className={cn(
              'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors',
              value.trim()
                ? 'bg-zinc-900 text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300'
                : 'bg-zinc-200 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-600',
            )}
            title="Enviar mensagem"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        )}
      </div>

      <p className="mt-1.5 text-center text-[11px] text-zinc-400">
        GoClaw pode cometer erros. Considere verificar informações importantes.
      </p>
    </div>
  )
}
