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
        'rounded-2xl border bg-primary shadow-sm transition-all',
        'focus-within:border-primary focus-within:shadow-md',
        'border-secondary'
      )}
    >
      {/* Textarea */}
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
          'w-full resize-none border-none bg-transparent px-4 pt-3.5 pb-2 text-[15px] text-primary',
          'placeholder:text-quaternary outline-none focus:ring-0',
          'disabled:opacity-50'
        )}
        style={{ boxShadow: 'none' }}
      />

      {/* Action bar */}
      <div className="flex items-center justify-between px-3 pb-3">
        {/* Left side - hint */}
        <span className="text-xs text-tertiary">
          {t('chatPage.enterToSend')}
        </span>

        {/* Right side - send/stop */}
        <div className="flex items-center gap-2">
          {isStreaming ? (
            <button
              onClick={() => onAbort?.()}
              className={cn(
                'flex h-8 w-8 cursor-pointer items-center justify-center rounded-full',
                'bg-error text-white transition-all hover:bg-red-600',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-error/50'
              )}
              aria-label="Stop generation"
            >
              <Square className="h-3.5 w-3.5" fill="currentColor" />
            </button>
          ) : (
            <button
              onClick={handleSend}
              disabled={!canSend}
              className={cn(
                'flex h-8 w-8 cursor-pointer items-center justify-center rounded-full transition-all',
                'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/50',
                canSend
                  ? 'bg-brand-solid text-white hover:bg-brand-hover'
                  : 'bg-secondary text-tertiary cursor-not-allowed'
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
