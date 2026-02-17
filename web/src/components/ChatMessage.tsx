import { memo, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Copy, Check, Terminal, ChevronDown, ChevronRight, Bot, User } from 'lucide-react'
import { cn } from '@/lib/utils'

interface ChatMessageProps {
  role: 'user' | 'assistant' | 'tool'
  content: string
  toolName?: string
  toolInput?: string
  isStreaming?: boolean
}

export const ChatMessage = memo(function ChatMessage({
  role, content, toolName, toolInput, isStreaming,
}: ChatMessageProps) {
  if (role === 'tool') {
    return <ToolMessage toolName={toolName} toolInput={toolInput} content={content} />
  }

  if (role === 'user') {
    return (
      <div className="flex gap-4 py-5 animate-fade-in">
        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-orange-500/20 to-amber-500/20 ring-1 ring-orange-500/20">
          <User className="h-5 w-5 text-orange-400" />
        </div>
        <div className="min-w-0 flex-1 pt-1">
          <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.15em] text-orange-400/80">Voce</p>
          <p className="whitespace-pre-wrap text-base leading-relaxed text-gray-200">{content}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex gap-4 py-5 animate-fade-in">
      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-500/20 to-green-500/20 ring-1 ring-emerald-500/20">
        <Bot className="h-5 w-5 text-emerald-400" />
      </div>
      <div className="min-w-0 flex-1 pt-1">
        <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.15em] text-emerald-400/80">DevClaw</p>
        <div className="prose prose-base max-w-none text-base leading-relaxed text-gray-300 prose-headings:text-white prose-headings:font-bold prose-strong:text-white prose-code:text-orange-400 prose-a:text-orange-400 prose-pre:bg-transparent prose-pre:p-0 prose-p:text-base prose-li:text-base">
          <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: CodeBlock }}>
            {content}
          </ReactMarkdown>
          {isStreaming && (
            <span className="ml-0.5 inline-block h-5 w-0.5 animate-cursor bg-orange-400 align-text-bottom" />
          )}
        </div>
      </div>
    </div>
  )
})

function ToolMessage({ toolName, toolInput, content }: { toolName?: string; toolInput?: string; content: string }) {
  const [expanded, setExpanded] = useState(false)
  return (
    <div className="ml-[60px] animate-fade-in py-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex cursor-pointer items-center gap-2.5 rounded-xl border border-white/[0.06] bg-[#111118] px-4 py-2.5 text-sm text-gray-400 transition-colors hover:bg-white/[0.04]"
      >
        <Terminal className="h-4 w-4 text-orange-500" />
        <span className="font-bold text-gray-300">{toolName || 'tool'}</span>
        {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
      </button>
      {expanded && (
        <div className="mt-2 overflow-hidden rounded-2xl border border-white/[0.06] bg-[#111118]">
          {toolInput && (
            <div className="border-b border-white/[0.04] px-5 py-4">
              <p className="mb-2 text-[10px] font-bold uppercase tracking-[0.15em] text-gray-600">Input</p>
              <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-sm text-gray-400">{toolInput}</pre>
            </div>
          )}
          <div className="px-5 py-4">
            <p className="mb-2 text-[10px] font-bold uppercase tracking-[0.15em] text-gray-600">Output</p>
            <pre className="max-h-60 overflow-x-auto overflow-y-auto whitespace-pre-wrap font-mono text-sm text-gray-400">{content}</pre>
          </div>
        </div>
      )}
    </div>
  )
}

function CodeBlock({ className, children, ...props }: React.HTMLAttributes<HTMLElement> & { children?: React.ReactNode }) {
  const [copied, setCopied] = useState(false)
  const isInline = !className

  if (isInline) {
    return (
      <code className="rounded-lg bg-white/[0.06] px-2 py-0.5 text-sm text-orange-400" {...props}>
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
    <div className="group relative not-prose my-4">
      {lang && (
        <div className="flex items-center justify-between rounded-t-2xl border border-b-0 border-white/[0.06] bg-[#111118] px-5 py-3">
          <span className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">{lang}</span>
          <button onClick={handleCopy} className="cursor-pointer text-gray-600 transition-colors hover:text-gray-300">
            {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
          </button>
        </div>
      )}
      <pre
        className={cn(
          'overflow-x-auto rounded-b-2xl border border-white/[0.06] bg-[#0a0a0f] p-5 text-sm leading-relaxed text-gray-300',
          !lang && 'rounded-t-2xl',
        )}
      >
        <code className={className} {...props}>{children}</code>
      </pre>
      {!lang && (
        <button
          onClick={handleCopy}
          className="absolute right-4 top-4 cursor-pointer rounded-xl p-2 text-gray-600 opacity-0 transition-all hover:bg-white/[0.06] hover:text-gray-300 group-hover:opacity-100"
        >
          {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
        </button>
      )}
    </div>
  )
}
