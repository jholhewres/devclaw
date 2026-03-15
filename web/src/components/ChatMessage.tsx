import { memo, useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
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
import { ThinkingBlock, extractThinkingContent } from '@/components/ThinkingBlock'

const MAX_PREVIEW = 50

function truncate(s: string, max: number): string {
  if (s.length <= max) return s
  return s.slice(0, max) + '\u2026'
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
        ? ` (${keys.slice(0, 2).map((k) => `${k}=${truncate(String(input[k]), 20)}`).join(', ')}${keys.length > 2 ? '\u2026' : ''})`
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
  isError?: boolean
}

export const ChatMessage = memo(function ChatMessage({
  role, content, toolName, toolInput, isStreaming, isError,
}: ChatMessageProps) {
  if (role === 'tool') {
    return <ToolMessage toolName={toolName} toolInput={toolInput} content={content} isError={isError} />
  }

  if (role === 'user') {
    return (
      <div className="flex items-start justify-end gap-3 animate-fade-in">
        <div className="max-w-[80%]">
          <div className="rounded-2xl rounded-tr-md bg-brand-subtle px-4 py-3">
            <p className="whitespace-pre-wrap text-[15px] leading-relaxed text-text-primary">
              {content}
            </p>
          </div>
        </div>
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand">
          <User className="h-4 w-4 text-white" />
        </div>
      </div>
    )
  }

  // Assistant message
  const isEmpty = !content || content.trim() === ''
  const { thinkingContent, cleanContent } = useMemo(
    () => extractThinkingContent(content || ''),
    [content]
  )

  return (
    <div
      className={cn(
        'group flex items-start gap-3',
        isStreaming ? 'animate-fade-in-up' : 'animate-fade-in',
      )}
    >
      <div
        className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-all duration-300',
          isStreaming
            ? 'bg-brand ring-2 ring-brand/30 ring-offset-2 ring-offset-bg-main'
            : 'bg-bg-elevated',
        )}
      >
        <Bot className={cn('h-4 w-4 transition-colors', isStreaming ? 'text-white' : 'text-text-muted')} />
      </div>
      <div className="relative min-w-0 flex-1">
        {isStreaming && isEmpty ? (
          <TypingDots />
        ) : (
          <>
            {/* Thinking block */}
            {thinkingContent && <ThinkingBlock content={thinkingContent} />}

            {/* Message body */}
            <div
              className={cn(
                'copilot-markdown text-[15px] leading-[1.7] text-text-primary',
                isStreaming && 'stream-shimmer',
              )}
            >
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ code: CodeBlock }}>
                {cleanContent}
              </ReactMarkdown>
              {isStreaming && (
                <span className="ml-0.5 inline-block h-[18px] w-[2px] rounded-full bg-brand align-text-bottom animate-pulse" />
              )}
            </div>
          </>
        )}

        {/* Copy button on hover for assistant messages */}
        {!isStreaming && !isEmpty && (
          <CopyMessageButton content={cleanContent} />
        )}
      </div>
    </div>
  )
})

/** Copy button that appears on hover */
function CopyMessageButton({ content }: { content: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard not available */
    }
  }

  return (
    <button
      onClick={handleCopy}
      className={cn(
        'absolute -bottom-6 left-0 flex items-center gap-1 rounded-md px-2 py-1 text-xs transition-all',
        'text-text-muted hover:text-text-primary hover:bg-bg-hover',
        'opacity-0 group-hover:opacity-100'
      )}
      aria-label={t('common.copy')}
    >
      {copied ? (
        <>
          <Check className="h-3 w-3 text-success" />
          <span className="text-success">{t('common.copied')}</span>
        </>
      ) : (
        <>
          <Copy className="h-3 w-3" />
          <span>{t('common.copy')}</span>
        </>
      )}
    </button>
  )
}

function TypingDots() {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-2 py-2">
      <div className="copilot-thinking-dots text-brand">
        <span />
        <span />
        <span />
      </div>
      <span className="text-sm text-text-muted">{t('chatPage.thinking')}</span>
    </div>
  )
}

function ToolMessage({ toolName, toolInput, content, isError }: { toolName?: string; toolInput?: string; content: string; isError?: boolean }) {
  const [expanded, setExpanded] = useState(false)
  const [activeTab, setActiveTab] = useState<'input' | 'output'>('output')
  const { summary, icon: Icon } = useMemo(
    () => getToolSummary(toolName || 'tool', toolInput),
    [toolName, toolInput],
  )

  const hasInput = !!toolInput?.trim()
  const hasOutput = !!content?.trim()
  const hasContent = hasInput || hasOutput

  return (
    <div className="ml-11 animate-fade-in py-1">
      <div className="rounded-xl border border-border bg-bg-subtle overflow-hidden">
        {/* Header */}
        <button
          onClick={() => setExpanded(!expanded)}
          className={cn(
            'flex w-full cursor-pointer items-center gap-2.5 px-3.5 py-2.5 text-left text-xs transition-colors',
            'hover:bg-bg-hover'
          )}
        >
          <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-brand-subtle">
            <Icon className="h-3.5 w-3.5 text-brand" />
          </div>
          <span className="min-w-0 flex-1 truncate font-medium text-text-primary">
            {summary}
          </span>
          {/* Status indicator */}
          <span className={cn('flex h-1.5 w-1.5 shrink-0 rounded-full', isError ? 'bg-error' : 'bg-success')} />
          {hasContent && (
            expanded
              ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-text-muted" />
              : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-text-muted" />
          )}
        </button>

        {/* Expanded content with tabs */}
        {expanded && hasContent && (
          <div className="border-t border-border">
            {/* Tabs */}
            {hasInput && hasOutput && (
              <div className="flex border-b border-border">
                <button
                  onClick={() => setActiveTab('input')}
                  className={cn(
                    'px-3.5 py-2 text-[11px] font-medium uppercase tracking-wider transition-colors',
                    activeTab === 'input'
                      ? 'text-brand border-b-2 border-brand -mb-px'
                      : 'text-text-muted hover:text-text-secondary'
                  )}
                >
                  Input
                </button>
                <button
                  onClick={() => setActiveTab('output')}
                  className={cn(
                    'px-3.5 py-2 text-[11px] font-medium uppercase tracking-wider transition-colors',
                    activeTab === 'output'
                      ? 'text-brand border-b-2 border-brand -mb-px'
                      : 'text-text-muted hover:text-text-secondary'
                  )}
                >
                  Output
                </button>
              </div>
            )}

            {/* Content */}
            <div className="px-3.5 py-3">
              {(activeTab === 'input' && hasInput) ? (
                <pre className="overflow-x-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-text-secondary">
                  {toolInput}
                </pre>
              ) : (
                <pre className="max-h-48 overflow-x-auto overflow-y-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-text-secondary">
                  {content}
                </pre>
              )}
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
      <code
        className="rounded-md bg-bg-subtle px-1.5 py-0.5 text-[13px] text-text-primary"
        {...props}
      >
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
    <div className="group/code relative not-prose my-3">
      {lang && (
        <div className="flex items-center justify-between rounded-t-xl border border-b-0 border-border bg-bg-subtle px-3 py-2">
          <span className="text-[10px] font-medium uppercase tracking-wider text-text-muted">
            {lang}
          </span>
          <button
            onClick={handleCopy}
            aria-label="Copy code"
            className="cursor-pointer text-text-muted transition-colors hover:text-text-primary"
          >
            {copied ? <Check className="h-3 w-3 text-success" /> : <Copy className="h-3 w-3" />}
          </button>
        </div>
      )}
      <pre
        className={cn(
          'overflow-x-auto border border-border bg-bg-subtle p-3 text-[13px] leading-relaxed text-text-primary',
          lang ? 'rounded-b-xl' : 'rounded-xl',
        )}
      >
        <code className={className} {...props}>{children}</code>
      </pre>
      {!lang && (
        <button
          onClick={handleCopy}
          aria-label="Copy code"
          className={cn(
            'absolute right-2 top-2 cursor-pointer rounded-xl p-1.5 transition-all',
            'text-text-muted hover:bg-bg-hover hover:text-text-primary',
            'opacity-0 group-hover/code:opacity-100'
          )}
        >
          {copied ? <Check className="h-3 w-3 text-success" /> : <Copy className="h-3 w-3" />}
        </button>
      )}
    </div>
  )
}
