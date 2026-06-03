import { useState, useRef, useEffect, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowUp, Square } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatInputProps {
  onSend: (message: string) => void
  onAbort?: () => void
  isStreaming?: boolean
  disabled?: boolean
  placeholder?: string
  rows?: number
  autoFocus?: boolean
}

export function ChatInput({
  onSend,
  onAbort,
  isStreaming = false,
  disabled = false,
  placeholder = 'Ask something or describe a task...',
  rows = 1,
  autoFocus = false,
}: ChatInputProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }, [value])

  useEffect(() => {
    if (autoFocus) textareaRef.current?.focus()
  }, [autoFocus])

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

  const canSend = value.trim().length > 0 && !disabled && !isStreaming

  return (
    <div
      className={cn(
        'rounded-xl border bg-secondary/50 shadow-sm transition-all',
        'focus-within:border-brand/40 focus-within:shadow-md',
        'border-secondary',
      )}
    >
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={disabled ? t('common.loading') : placeholder}
        disabled={disabled}
        rows={rows}
        autoFocus={autoFocus}
        className={cn(
          'w-full resize-none border-none bg-transparent px-4 pt-3.5 pb-1 text-[16px] text-primary',
          'placeholder:text-quaternary outline-hidden focus:ring-0',
          'disabled:opacity-50',
        )}
        style={{ boxShadow: 'none' }}
      />

      <div className="flex items-center justify-between px-3 pb-2.5">
        <span className="text-[11px] text-quaternary">
          {t('chatPage.enterToSend')}
        </span>

        <div className="flex items-center gap-2">
          {isStreaming ? (
            <button
              onClick={() => onAbort?.()}
              className={cn(
                'flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg',
                'bg-error/10 text-error border border-error/20 transition-all hover:bg-error/20',
                'focus-visible:outline-hidden',
              )}
              aria-label="Stop generation"
            >
              <Square className="h-3 w-3" fill="currentColor" />
            </button>
          ) : (
            <button
              onClick={handleSend}
              disabled={!canSend}
              className={cn(
                'flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg transition-all',
                'focus-visible:outline-hidden',
                canSend
                  ? 'bg-brand-solid text-white hover:bg-brand-solid_hover'
                  : 'bg-secondary text-quaternary',
              )}
              aria-label="Send message"
            >
              <ArrowUp className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
