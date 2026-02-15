import { memo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Copy, Check, Terminal, ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useState } from 'react'

interface ChatMessageProps {
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolName?: string
  toolInput?: string
  isStreaming?: boolean
}

/**
 * Renderiza uma mensagem de chat.
 * - User: bolha alinhada à direita
 * - Assistant: texto à esquerda com markdown
 * - Tool: card colapsável
 */
export const ChatMessage = memo(function ChatMessage({
  role,
  content,
  toolName,
  toolInput,
  isStreaming,
}: ChatMessageProps) {
  if (role === 'tool') {
    return <ToolMessage toolName={toolName} toolInput={toolInput} content={content} />
  }

  if (role === 'user') {
    return (
      <div className="flex justify-end animate-fade-in">
        <div className="max-w-[75%] rounded-2xl rounded-br-md bg-zinc-900 px-4 py-2.5 text-sm text-white dark:bg-zinc-100 dark:text-zinc-900">
          <p className="whitespace-pre-wrap">{content}</p>
        </div>
      </div>
    )
  }

  /* Assistant */
  return (
    <div className="animate-fade-in">
      <div className="max-w-[85%]">
        <div className="prose prose-sm prose-zinc dark:prose-invert max-w-none text-sm leading-relaxed">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              code: CodeBlock,
            }}
          >
            {content}
          </ReactMarkdown>
          {isStreaming && (
            <span className="inline-block h-4 w-0.5 bg-zinc-800 dark:bg-zinc-200 animate-cursor ml-0.5 align-text-bottom" />
          )}
        </div>
      </div>
    </div>
  )
})

/* ── Tool Message ── */

function ToolMessage({
  toolName,
  toolInput,
  content,
}: {
  toolName?: string
  toolInput?: string
  content: string
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="animate-fade-in">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2 rounded-lg border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-600 hover:bg-zinc-100 transition-colors dark:border-zinc-700 dark:bg-zinc-800/50 dark:text-zinc-400 dark:hover:bg-zinc-800"
      >
        <Terminal className="h-3.5 w-3.5" />
        <span className="font-medium">{toolName || 'tool'}</span>
        {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
      </button>

      {expanded && (
        <div className="mt-1 ml-2 rounded-lg border border-zinc-200 bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-800/50 overflow-hidden">
          {toolInput && (
            <div className="border-b border-zinc-200 dark:border-zinc-700 px-3 py-2">
              <p className="text-[10px] uppercase tracking-wider text-zinc-400 mb-1">Input</p>
              <pre className="text-xs text-zinc-600 dark:text-zinc-400 overflow-x-auto whitespace-pre-wrap">
                {toolInput}
              </pre>
            </div>
          )}
          <div className="px-3 py-2">
            <p className="text-[10px] uppercase tracking-wider text-zinc-400 mb-1">Output</p>
            <pre className="text-xs text-zinc-600 dark:text-zinc-400 overflow-x-auto whitespace-pre-wrap max-h-48 overflow-y-auto">
              {content}
            </pre>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── Code Block ── */

function CodeBlock({
  className,
  children,
  ...props
}: React.HTMLAttributes<HTMLElement> & { children?: React.ReactNode }) {
  const [copied, setCopied] = useState(false)
  const isInline = !className

  if (isInline) {
    return (
      <code
        className="rounded bg-zinc-100 px-1.5 py-0.5 text-[13px] dark:bg-zinc-800"
        {...props}
      >
        {children}
      </code>
    )
  }

  const text = String(children).replace(/\n$/, '')
  const lang = className?.replace('language-', '') || ''

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="group relative not-prose">
      {lang && (
        <div className="flex items-center justify-between rounded-t-lg border border-b-0 border-zinc-200 bg-zinc-100 px-3 py-1 dark:border-zinc-700 dark:bg-zinc-800">
          <span className="text-[11px] text-zinc-500">{lang}</span>
          <button
            onClick={handleCopy}
            className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300 transition-colors"
          >
            {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
          </button>
        </div>
      )}
      <pre
        className={cn(
          'overflow-x-auto rounded-b-lg border border-zinc-200 bg-zinc-950 p-3 text-[13px] text-zinc-100 dark:border-zinc-700',
          !lang && 'rounded-t-lg',
        )}
      >
        <code className={className} {...props}>
          {children}
        </code>
      </pre>
      {!lang && (
        <button
          onClick={handleCopy}
          className="absolute right-2 top-2 rounded p-1 text-zinc-500 opacity-0 group-hover:opacity-100 hover:text-zinc-300 transition-all"
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      )}
    </div>
  )
}
