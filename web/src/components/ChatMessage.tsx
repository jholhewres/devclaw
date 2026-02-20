import { memo, useState, useMemo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Copy,
  Check,
  Terminal,
  ChevronDown,
  ChevronRight,
  Bot,
  User,
  FileText,
  FileEdit,
  Search,
  Globe,
  Database,
  Lock,
  Wrench,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const MAX_PREVIEW = 50

function truncate(s: string, max: number): string {
  if (s.length <= max) return s
  return s.slice(0, max) + '…'
}

function toTildePath(path: string): string {
  const m = path.match(/^\/home\/[^/]+\/(.*)$/)
  return m ? `~/${m[1]}` : path
}

interface ToolSummary {
  summary: string
  icon: LucideIcon
}

function getToolSummary(toolName: string, toolInput: string | undefined): ToolSummary {
  const fallback: ToolSummary = {
    summary: `Using ${toolName}`,
    icon: Wrench,
  }
  if (!toolInput?.trim()) return fallback

  let input: Record<string, unknown>
  try {
    input = JSON.parse(toolInput) as Record<string, unknown>
  } catch {
    return fallback
  }

  const path = input.path as string | undefined
  const command = input.command as string | undefined
  const query = input.query as string | undefined
  const url = input.url as string | undefined
  const name = input.name as string | undefined
  const content = input.content as string | undefined

  switch (toolName) {
    case 'read_file':
      return { summary: path ? `Reading ${toTildePath(path)}` : 'Reading file', icon: FileText }
    case 'write_file':
      return { summary: path ? `Writing ${toTildePath(path)}` : 'Writing file', icon: FileEdit }
    case 'edit_file':
      return { summary: path ? `Editing ${toTildePath(path)}` : 'Editing file', icon: FileEdit }
    case 'bash':
    case 'exec':
      return { summary: command ? `Running: ${truncate(command, MAX_PREVIEW)}` : 'Running command', icon: Terminal }
    case 'web_search':
      return { summary: query ? `Searching: ${truncate(query, MAX_PREVIEW)}` : 'Searching the web', icon: Search }
    case 'web_fetch':
      return { summary: url ? `Fetching: ${truncate(url, MAX_PREVIEW)}` : 'Fetching URL', icon: Globe }
    case 'memory_save':
      return { summary: content ? `Saving to memory: ${truncate(content, MAX_PREVIEW)}` : 'Saving to memory', icon: Database }
    case 'memory_search':
      return { summary: query ? `Searching memory: ${truncate(query, MAX_PREVIEW)}` : 'Searching memory', icon: Database }
    case 'vault_save':
      return { summary: name ? `Vault: save ${name}` : 'Vault: save', icon: Lock }
    case 'vault_get':
      return { summary: name ? `Vault: get ${name}` : 'Vault: get', icon: Lock }
    case 'vault_list':
      return { summary: 'Vault: list', icon: Lock }
    case 'vault_delete':
      return { summary: name ? `Vault: delete ${name}` : 'Vault: delete', icon: Lock }
    default: {
      const keys = Object.keys(input).filter((k) => input[k] !== undefined && input[k] !== '')
      const preview = keys.length > 0
        ? ` (${keys.slice(0, 2).map((k) => `${k}=${truncate(String(input[k]), 20)}`).join(', ')}${keys.length > 2 ? '…' : ''})`
        : ''
      return { summary: `Using ${toolName}${preview}`, icon: Wrench }
    }
  }
}

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
      <div className="flex items-start justify-end gap-3 animate-fade-in">
        <div className="max-w-[75%]">
          <div className="rounded-2xl rounded-tr-md bg-blue-600 px-4 py-3">
            <p className="whitespace-pre-wrap text-[15px] leading-[1.6] text-white">{content}</p>
          </div>
        </div>
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-zinc-800">
          <User className="h-3.5 w-3.5 text-zinc-400" />
        </div>
      </div>
    )
  }

  const isEmpty = !content || content.trim() === ''

  return (
    <div className={cn(
      'flex items-start gap-3',
      isStreaming ? 'animate-fade-in-up' : 'animate-fade-in',
    )}>
      <div className={cn(
        'flex h-7 w-7 shrink-0 items-center justify-center rounded-full transition-colors',
        isStreaming ? 'bg-emerald-500/15' : 'bg-zinc-800',
      )}>
        <Bot className="h-3.5 w-3.5 text-emerald-400" />
      </div>
      <div className="min-w-0 flex-1">
        {isStreaming && isEmpty ? (
          <TypingDots />
        ) : (
          <div className={cn(
            'prose prose-sm max-w-none text-[15px] leading-[1.7] text-zinc-300',
            'prose-headings:text-white prose-headings:font-bold prose-strong:text-white',
            'prose-code:text-blue-400 prose-a:text-blue-400',
            'prose-pre:bg-transparent prose-pre:p-0',
            'prose-p:text-[15px] prose-li:text-[15px] prose-p:my-2 prose-p:first:mt-0 prose-p:last:mb-0',
            isStreaming && 'stream-shimmer',
          )}>
            <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: CodeBlock }}>
              {content}
            </ReactMarkdown>
            {isStreaming && (
              <span className="ml-0.5 inline-block h-[18px] w-[2px] animate-cursor rounded-full bg-emerald-400 align-text-bottom" />
            )}
          </div>
        )}
      </div>
    </div>
  )
})

function TypingDots() {
  return (
    <div className="flex items-center gap-[3px] py-1">
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-blue-400" style={{ animationDelay: '0ms' }} />
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-blue-400" style={{ animationDelay: '150ms' }} />
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-blue-400" style={{ animationDelay: '300ms' }} />
    </div>
  )
}

function ToolMessage({ toolName, toolInput, content }: { toolName?: string; toolInput?: string; content: string }) {
  const [expanded, setExpanded] = useState(false)
  const { summary, icon: Icon } = useMemo(
    () => getToolSummary(toolName || 'tool', toolInput),
    [toolName, toolInput],
  )
  return (
    <div className="ml-10 animate-fade-in py-1">
      <div className="rounded-lg border-l-2 border-blue-500/40 bg-zinc-800/30">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-xs text-zinc-400 transition-colors hover:bg-zinc-800/50"
        >
          <Icon className="h-3.5 w-3.5 shrink-0 text-blue-400" />
          <span className="min-w-0 flex-1 font-medium text-zinc-300">{summary}</span>
          {expanded ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
        </button>
        {expanded && (
          <div className="border-t border-zinc-700/20">
            {toolInput && (
              <div className="border-b border-zinc-700/20 px-3 py-2">
                <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-zinc-500">Input</p>
                <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-[11px] text-zinc-400">{toolInput}</pre>
              </div>
            )}
            <div className="px-3 py-2">
              <p className="mb-1 text-[10px] font-medium uppercase tracking-wider text-zinc-500">Output</p>
              <pre className="max-h-48 overflow-x-auto overflow-y-auto whitespace-pre-wrap font-mono text-[11px] text-zinc-400">{content}</pre>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function CodeBlock({ className, children, ...props }: React.HTMLAttributes<HTMLElement> & { children?: React.ReactNode }) {
  const [copied, setCopied] = useState(false)
  const isInline = !className

  if (isInline) {
    return (
      <code className="rounded-md bg-zinc-800 px-1.5 py-0.5 text-[13px] text-blue-400" {...props}>
        {children}
      </code>
    )
  }

  const text = String(children).replace(/\n$/, '')
  const lang = className?.replace('language-', '') || ''

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard not available */
    }
  }

  return (
    <div className="group relative not-prose my-3">
      {lang && (
        <div className="flex items-center justify-between rounded-t-xl border border-b-0 border-zinc-700/30 bg-zinc-800/60 px-3 py-2">
          <span className="text-[10px] font-medium uppercase tracking-wider text-zinc-500">{lang}</span>
          <button onClick={handleCopy} aria-label="Copiar código" className="cursor-pointer text-zinc-600 transition-colors hover:text-zinc-300">
            {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          </button>
        </div>
      )}
      <pre
        className={cn(
          'overflow-x-auto border border-zinc-700/30 bg-zinc-900 p-3 text-[13px] leading-relaxed text-zinc-300',
          lang ? 'rounded-b-xl' : 'rounded-xl',
        )}
      >
        <code className={className} {...props}>{children}</code>
      </pre>
      {!lang && (
        <button
          onClick={handleCopy}
          aria-label="Copiar código"
          className="absolute right-2 top-2 cursor-pointer rounded-lg p-1.5 text-zinc-600 opacity-0 transition-all hover:bg-zinc-800 hover:text-zinc-300 group-hover:opacity-100"
        >
          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
        </button>
      )}
    </div>
  )
}
