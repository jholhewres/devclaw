import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bot,
  Plus,
  Star,
  Trash2,
  Save,
  X,
  Users,
  ToggleLeft,
  ToggleRight,
  Hash,
  Sparkles,
  Network,
  Wrench,
  FileText,
  Folder,
  Search,
  BookOpen,
  Loader2,
  CheckCircle2,
} from 'lucide-react'
import {
  api,
  type AgentInfo,
  type AgentIdentity,
  type CreateAgentRequest,
  type UpdateAgentRequest,
  type ChannelHealth,
  type SkillInfo,
  type ToolProfileInfo,
  type ModelInfo,
} from '@/lib/api'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { SearchInput } from '@/components/ui/SearchInput'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { Tabs } from '@/components/ui/Tabs'
import { Select } from '@/components/ui/Select'
import { StatusDot } from '@/components/ui/StatusDot'
import { Toggle } from '@/components/ui/Toggle'
import { LoadingSpinner, ErrorState } from '@/components/ui/ConfigComponents'
import { UnsavedChangesBar } from '@/components/ui/UnsavedChangesBar'

// ── Constants ──

const LANGUAGE_OPTIONS = [
  { value: '', label: 'Global default' },
  { value: 'pt-BR', label: 'Português (Brasil)' },
  { value: 'pt-PT', label: 'Português (Portugal)' },
  { value: 'en-US', label: 'English (US)' },
  { value: 'en-GB', label: 'English (UK)' },
  { value: 'es-ES', label: 'Español (España)' },
  { value: 'es-MX', label: 'Español (México)' },
  { value: 'fr-FR', label: 'Français' },
  { value: 'de-DE', label: 'Deutsch' },
  { value: 'it-IT', label: 'Italiano' },
  { value: 'ja-JP', label: '日本語' },
  { value: 'ko-KR', label: '한국어' },
  { value: 'zh-CN', label: '中文 (简体)' },
  { value: 'zh-TW', label: '中文 (繁體)' },
]

const TIMEZONE_OPTIONS = [
  { value: '', label: 'Global default' },
  { value: 'America/Sao_Paulo', label: 'São Paulo (GMT-3)' },
  { value: 'America/New_York', label: 'New York (GMT-5)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (GMT-8)' },
  { value: 'Europe/London', label: 'London (GMT+0)' },
  { value: 'Europe/Paris', label: 'Paris (GMT+1)' },
  { value: 'Europe/Berlin', label: 'Berlin (GMT+1)' },
  { value: 'Asia/Tokyo', label: 'Tokyo (GMT+9)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (GMT+8)' },
  { value: 'Asia/Dubai', label: 'Dubai (GMT+4)' },
  { value: 'Australia/Sydney', label: 'Sydney (GMT+11)' },
  { value: 'UTC', label: 'UTC' },
]

const SOUL_TEMPLATE = `# Core Truths
- Be genuinely helpful, not performatively helpful
- Have opinions — an assistant with no personality is just a search engine
- Be resourceful before asking — try to figure it out first

# Boundaries
- Private things stay private
- When in doubt, ask before acting externally
- Never send half-baked replies

# Vibe
Be the assistant you'd actually want to talk to.
Concise when needed, thorough when it matters.`

const inputCls =
  'w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid'
const textareaCls = `${inputCls} resize-y`

// ── Shared data hook ──

function useSharedData() {
  const [models, setModels] = useState<ModelInfo[]>([])
  const [channels, setChannels] = useState<ChannelHealth[]>([])
  const [skills, setSkills] = useState<SkillInfo[]>([])
  const [toolProfiles, setToolProfiles] = useState<ToolProfileInfo[]>([])
  const [toolGroups, setToolGroups] = useState<Record<string, string[]>>({})
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    Promise.all([
      api.models.list().catch(() => [] as ModelInfo[]),
      api.channels.list().catch(() => [] as ChannelHealth[]),
      api.skills.list().catch(() => [] as SkillInfo[]),
      api.settings.toolProfiles.list().catch(() => ({ profiles: [] as ToolProfileInfo[], groups: {} as Record<string, string[]> })),
    ]).then(([m, ch, sk, tp]) => {
      setModels(m)
      setChannels(ch)
      setSkills(sk)
      setToolProfiles(tp.profiles)
      setToolGroups(tp.groups)
      setLoaded(true)
    })
  }, [])

  const modelOptions = useMemo(() => {
    const opts = [{ value: '', label: 'Global default' }]
    for (const m of models) {
      opts.push({ value: m.id, label: m.name || m.id })
    }
    return opts
  }, [models])

  return { models, channels, skills, toolProfiles, toolGroups, modelOptions, loaded }
}

// ── Main Page ──

export function Agents() {
  const { t } = useTranslation()
  const [agents, setAgents] = useState<AgentInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const shared = useSharedData()

  useEffect(() => {
    api.agents
      .list()
      .then(setAgents)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!actionError) return
    const timer = setTimeout(() => setActionError(null), 3000)
    return () => clearTimeout(timer)
  }, [actionError])

  const filtered = useMemo(
    () =>
      agents.filter(
        (a) =>
          a.name.toLowerCase().includes(search.toLowerCase()) ||
          a.description?.toLowerCase().includes(search.toLowerCase()) ||
          a.id.toLowerCase().includes(search.toLowerCase()),
      ),
    [agents, search],
  )

  const handleToggle = useCallback(async (id: string, currentActive: boolean) => {
    setActionError(null)
    try {
      await api.agents.toggle(id, !currentActive)
      setAgents((prev) =>
        prev.map((a) => (a.id === id ? { ...a, active: !currentActive } : a)),
      )
    } catch {
      setActionError(id)
    }
  }, [])

  const handleSetDefault = useCallback(async (id: string) => {
    setActionError(null)
    try {
      await api.agents.setDefault(id)
      setAgents((prev) =>
        prev.map((a) => ({ ...a, default: a.id === id })),
      )
    } catch {
      setActionError(id)
    }
  }, [])

  const handleDelete = useCallback(async (id: string) => {
    if (!window.confirm(t('agentsPage.deleteConfirm'))) return
    try {
      await api.agents.delete(id)
      setAgents((prev) => prev.filter((a) => a.id !== id))
      setSelectedId((prev) => (prev === id ? null : prev))
    } catch {
      setActionError(id)
    }
  }, [])

  const handleCreated = (agent: AgentInfo) => {
    setAgents((prev) => [...prev, agent])
    setShowCreate(false)
  }

  const activeCount = agents.filter((a) => a.active).length
  const selected = agents.find((a) => a.id === selectedId)

  if (loading) return <LoadingSpinner />
  if (loadError) {
    return (
      <ErrorState
        message={t('common.error')}
        onRetry={() => window.location.reload()}
        retryLabel={t('common.retry')}
      />
    )
  }

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8">
      <div className="flex items-center justify-between">
        <PageHeader
          title={t('agentsPage.title')}
          description={t('agentsPage.activeCount', { active: activeCount, total: agents.length })}
        />
        <Button
          onClick={() => setShowCreate(!showCreate)}
          variant={showCreate ? 'ghost' : 'default'}
        >
          {showCreate ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {showCreate ? t('common.cancel') : t('agentsPage.newAgent')}
        </Button>
      </div>

      {showCreate && (
        <CreateAgentPanel
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
          shared={shared}
        />
      )}

      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder={t('agentsPage.searchPlaceholder')}
        className="mt-6"
      />

      <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {filtered.map((agent) => {
          const isPlugin = agent.source === 'plugin'
          return (
            <Card
              key={agent.id}
              padding="lg"
              className={cn(
                'group relative overflow-hidden transition-all cursor-pointer',
                agent.active ? 'border-brand/30' : 'hover:border-primary',
                selectedId === agent.id && 'ring-2 ring-brand',
              )}
              onClick={() => setSelectedId(selectedId === agent.id ? null : agent.id)}
            >
              {/* Status badges */}
              <div className="absolute right-4 top-4 flex items-center gap-1.5">
                {agent.default && (
                  <Badge className="bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                    {t('agentsPage.default')}
                  </Badge>
                )}
                {isPlugin && (
                  <Badge className="bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400">
                    {t('agentsPage.plugin')}
                  </Badge>
                )}
              </div>

              {/* Icon */}
              <div
                className={cn(
                  'flex h-14 w-14 items-center justify-center rounded-xl text-2xl transition-colors',
                  agent.active
                    ? 'bg-brand-secondary text-brand-tertiary'
                    : 'bg-secondary text-tertiary group-hover:text-secondary',
                )}
              >
                {agent.identity?.emoji ? (
                  <span>{agent.identity.emoji}</span>
                ) : (
                  <Bot className="h-7 w-7" />
                )}
              </div>

              {/* Content */}
              <h3 className="mt-4 text-lg font-semibold text-primary flex items-center gap-1.5">
                {agent.name}
                {agent.file_backed && (
                  <span className="text-tertiary" title="File-backed workspace">
                    <Folder className="h-3.5 w-3.5" />
                  </span>
                )}
                {agent.identity?.creature && (
                  <span className="ml-2 text-sm font-normal text-tertiary">
                    {agent.identity.creature}
                  </span>
                )}
              </h3>
              {agent.model && (
                <p className="mt-0.5 text-xs text-tertiary">{agent.model}</p>
              )}
              {agent.description && (
                <p className="mt-2 text-sm leading-relaxed text-secondary line-clamp-2">
                  {agent.description}
                </p>
              )}
              {agent.soul && (
                <p className="mt-1 text-xs italic text-quaternary line-clamp-1">
                  {agent.soul.slice(0, 80)}{agent.soul.length > 80 ? '...' : ''}
                </p>
              )}

              {actionError === agent.id && (
                <p className="mt-2 text-xs text-fg-error-secondary">{t('common.error')}</p>
              )}

              {/* Metadata badges */}
              <div className="mt-4 flex items-center justify-between">
                <div className="flex items-center gap-2 flex-wrap">
                  {(agent.channels?.length ?? 0) > 0 && (
                    <span className="flex items-center gap-1 rounded-full bg-secondary px-2.5 py-0.5 text-xs font-medium text-tertiary">
                      <Hash className="h-3 w-3" />
                      {agent.channels?.join(', ')}
                    </span>
                  )}
                  {agent.member_count > 0 && (
                    <span className="flex items-center gap-1 rounded-full bg-secondary px-2.5 py-0.5 text-xs font-medium text-tertiary">
                      <Users className="h-3 w-3" />
                      {agent.member_count}
                    </span>
                  )}
                </div>

                {/* Actions */}
                {!isPlugin && (
                  <div className="flex items-center gap-1">
                    {!agent.default && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          handleSetDefault(agent.id)
                        }}
                        className="cursor-pointer text-tertiary transition-colors hover:text-amber-500 opacity-0 group-hover:opacity-100"
                        title="Set as default"
                      >
                        <Star className="h-4 w-4" />
                      </button>
                    )}
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        handleDelete(agent.id)
                      }}
                      className="cursor-pointer text-tertiary transition-colors hover:text-fg-error-secondary opacity-0 group-hover:opacity-100"
                      title="Delete agent"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        handleToggle(agent.id, agent.active)
                      }}
                      className="cursor-pointer text-tertiary transition-colors hover:text-primary"
                    >
                      {agent.active ? (
                        <ToggleRight className="h-7 w-7 text-brand-tertiary" />
                      ) : (
                        <ToggleLeft className="h-7 w-7" />
                      )}
                    </button>
                  </div>
                )}
              </div>
            </Card>
          )
        })}
      </div>

      {filtered.length === 0 && (
        <EmptyState
          icon={<Bot className="h-6 w-6" />}
          title={search ? t('agentsPage.noResults') : t('agentsPage.noAgents')}
          description={!search ? t('agentsPage.noAgentsDesc') : undefined}
        />
      )}

      {/* Detail/Edit panel — key forces remount when agent changes */}
      {selected && selected.source !== 'plugin' && (
        <AgentDetailPanel
          key={selected.id}
          agent={selected}
          onClose={() => setSelectedId(null)}
          onSaved={(updated) =>
            setAgents((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
          }
          onDeleted={(id) => {
            setAgents((prev) => prev.filter((a) => a.id !== id))
            setSelectedId(null)
          }}
          shared={shared}
        />
      )}
    </div>
  )
}

// ── Create Agent Panel ──

interface SharedData {
  models: ModelInfo[]
  channels: ChannelHealth[]
  skills: SkillInfo[]
  toolProfiles: ToolProfileInfo[]
  toolGroups: Record<string, string[]>
  modelOptions: { value: string; label: string }[]
  loaded: boolean
}

function CreateAgentPanel({
  onCreated,
  onCancel,
  shared,
}: {
  onCreated: (agent: AgentInfo) => void
  onCancel: () => void
  shared: SharedData
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [model, setModel] = useState('')
  const [emoji, setEmoji] = useState('')
  const [channels, setChannels] = useState<string[]>([])
  const [selectedSkills, setSelectedSkills] = useState<string[]>([])
  const [soul, setSoul] = useState('')
  const [showSoul, setShowSoul] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const toggleChannel = (ch: string) => {
    setChannels((prev) =>
      prev.includes(ch) ? prev.filter((c) => c !== ch) : [...prev, ch],
    )
  }

  const toggleSkill = (name: string) => {
    setSelectedSkills((prev) =>
      prev.includes(name) ? prev.filter((s) => s !== name) : [...prev, name],
    )
  }

  const handleCreate = async () => {
    if (!name.trim()) return
    setCreating(true)
    setError(null)
    try {
      const req: CreateAgentRequest = {
        name: name.trim(),
        description: description.trim() || undefined,
        model: model || undefined,
        identity: emoji.trim() ? { emoji: emoji.trim() } : undefined,
        channels: channels.length > 0 ? channels : undefined,
        skills: selectedSkills.length > 0 ? selectedSkills : undefined,
        soul: soul.trim() || undefined,
      }
      const result = await api.agents.create(req)
      const created = await api.agents.get(result.id)
      onCreated(created)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create agent')
    } finally {
      setCreating(false)
    }
  }

  // Top skills for quick-select (first 8 enabled ones)
  const topSkills = shared.skills.filter((s) => s.enabled).slice(0, 8)

  return (
    <Card padding="lg" className="mt-4 border-brand/30">
      <h3 className="text-sm font-semibold text-primary">New Agent</h3>
      <p className="mt-1 text-xs text-tertiary">
        Create a new agent with its own instructions and settings
      </p>
      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Name *</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            placeholder="e.g. Support Agent"
            className={inputCls}
            disabled={creating}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Emoji</label>
          <input
            type="text"
            value={emoji}
            onChange={(e) => setEmoji(e.target.value)}
            placeholder="e.g. 🦊"
            className={inputCls}
            disabled={creating}
          />
        </div>
        <Select
          label="Model"
          value={model}
          onChange={setModel}
          options={shared.modelOptions}
          disabled={creating}
        />
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Description</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Short description"
            className={inputCls}
            disabled={creating}
          />
        </div>
      </div>

      {/* Channel selection */}
      {shared.channels.length > 0 && (
        <div className="mt-4 space-y-1.5">
          <label className="text-sm font-medium text-primary">Channels</label>
          <div className="flex flex-wrap gap-2">
            {shared.channels.map((ch) => (
              <button
                key={ch.name}
                type="button"
                onClick={() => toggleChannel(ch.name)}
                disabled={creating}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm transition-colors cursor-pointer',
                  channels.includes(ch.name)
                    ? 'border-brand bg-brand-secondary text-brand-tertiary'
                    : 'border-secondary text-secondary hover:border-primary',
                )}
              >
                <StatusDot
                  status={ch.connected ? 'online' : ch.configured ? 'offline' : 'warning'}
                />
                {ch.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Quick skill select */}
      {topSkills.length > 0 && (
        <div className="mt-4 space-y-1.5">
          <label className="text-sm font-medium text-primary">Skills</label>
          <div className="flex flex-wrap gap-2">
            {topSkills.map((sk) => (
              <button
                key={sk.name}
                type="button"
                onClick={() => toggleSkill(sk.name)}
                disabled={creating}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm transition-colors cursor-pointer',
                  selectedSkills.includes(sk.name)
                    ? 'border-brand bg-brand-secondary text-brand-tertiary'
                    : 'border-secondary text-secondary hover:border-primary',
                )}
              >
                <BookOpen className="h-3 w-3" />
                {sk.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Collapsible Soul */}
      <details open={showSoul} onToggle={(e) => setShowSoul((e.target as HTMLDetailsElement).open)} className="mt-4">
        <summary className="cursor-pointer text-sm font-medium text-secondary hover:text-primary">
          Soul (persona)
        </summary>
        <textarea
          value={soul}
          onChange={(e) => setSoul(e.target.value)}
          rows={6}
          placeholder="Define personality traits, boundaries, vibe..."
          className={cn(textareaCls, 'mt-2 font-mono text-xs')}
          disabled={creating}
        />
      </details>

      {error && <p className="mt-3 text-xs text-fg-error-secondary">{error}</p>}
      <div className="mt-4 flex items-center gap-2">
        <Button onClick={handleCreate} disabled={creating || !name.trim()}>
          {creating ? t('common.saving') : 'Create'}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
      </div>
    </Card>
  )
}

// ── Chip Input ──

function ChipInput({
  value,
  onChange,
  placeholder,
  suggestions,
}: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder?: string
  suggestions?: string[]
}) {
  const [input, setInput] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const addChip = (text: string) => {
    const trimmed = text.trim()
    if (trimmed && !value.includes(trimmed)) {
      onChange([...value, trimmed])
    }
    setInput('')
  }

  const removeChip = (idx: number) => {
    onChange(value.filter((_, i) => i !== idx))
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.key === 'Enter' || e.key === ',') && input.trim()) {
      e.preventDefault()
      addChip(input)
    } else if (e.key === 'Backspace' && !input && value.length > 0) {
      removeChip(value.length - 1)
    }
  }

  // Filter suggestions not already selected
  const filteredSuggestions = suggestions?.filter(
    (s) => !value.includes(s) && s.toLowerCase().includes(input.toLowerCase()),
  )

  return (
    <div
      className="flex flex-wrap items-center gap-1.5 rounded-lg border border-secondary bg-primary px-2 py-1.5 min-h-[38px] cursor-text"
      onClick={() => inputRef.current?.focus()}
    >
      {value.map((chip, idx) => (
        <span
          key={chip}
          className="inline-flex items-center gap-1 rounded-md bg-brand-secondary px-2 py-0.5 text-xs font-medium text-brand-tertiary"
        >
          {chip}
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              removeChip(idx)
            }}
            className="cursor-pointer text-brand-tertiary/60 hover:text-brand-tertiary"
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      ))}
      <div className="relative flex-1 min-w-[100px]">
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          onBlur={() => input.trim() && addChip(input)}
          placeholder={value.length === 0 ? placeholder : ''}
          className="w-full bg-transparent text-sm text-primary outline-none placeholder:text-quaternary"
        />
        {input && filteredSuggestions && filteredSuggestions.length > 0 && (
          <div className="absolute left-0 top-full z-50 mt-1 max-h-32 w-48 overflow-auto rounded-lg border border-secondary bg-primary shadow-lg">
            {filteredSuggestions.slice(0, 8).map((s) => (
              <button
                key={s}
                type="button"
                onMouseDown={(e) => {
                  e.preventDefault()
                  addChip(s)
                }}
                className="w-full cursor-pointer px-3 py-1.5 text-left text-xs text-primary hover:bg-secondary"
              >
                {s}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Agent Detail Panel (Tabbed) ──

type DetailTab = 'overview' | 'soul_identity' | 'tools' | 'skills' | 'routing' | 'files'

const BOOTSTRAP_FILES = ['SOUL.md', 'IDENTITY.md', 'TOOLS.md', 'MEMORY.md', 'AGENTS.md', 'HEARTBEAT.md']

interface DetailValues {
  name: string
  description: string
  model: string
  instructions: string
  soul: string
  language: string
  channels: string[]
  tool_profile: string
  tools_allow: string[]
  tools_deny: string[]
  max_turns: number
  run_timeout: number
  skills: string[]
  trigger: string
  timezone: string
  members: string
  groups: string
  identity: {
    name: string
    emoji: string
    creature: string
    vibe: string
    theme: string
    avatar: string
  }
}

function agentToValues(agent: AgentInfo): DetailValues {
  return {
    name: agent.name,
    description: agent.description ?? '',
    model: agent.model ?? '',
    instructions: agent.instructions ?? '',
    soul: agent.soul ?? '',
    language: agent.language ?? '',
    channels: agent.channels ?? [],
    tool_profile: agent.tool_profile ?? '',
    tools_allow: agent.tools_allow ?? [],
    tools_deny: agent.tools_deny ?? [],
    max_turns: agent.max_turns ?? 0,
    run_timeout: agent.run_timeout ?? 0,
    skills: agent.skills ?? [],
    trigger: agent.trigger ?? '',
    timezone: agent.timezone ?? '',
    members: (agent.members ?? []).join('\n'),
    groups: (agent.groups ?? []).join('\n'),
    identity: {
      name: agent.identity?.name ?? '',
      emoji: agent.identity?.emoji ?? '',
      creature: agent.identity?.creature ?? '',
      vibe: agent.identity?.vibe ?? '',
      theme: agent.identity?.theme ?? '',
      avatar: agent.identity?.avatar ?? '',
    },
  }
}

function valuesToUpdate(v: DetailValues): UpdateAgentRequest {
  const identity: AgentIdentity = {
    name: v.identity.name || undefined,
    emoji: v.identity.emoji || undefined,
    creature: v.identity.creature || undefined,
    vibe: v.identity.vibe || undefined,
    theme: v.identity.theme || undefined,
    avatar: v.identity.avatar || undefined,
  }
  const hasIdentity = Object.values(identity).some(Boolean)

  return {
    name: v.name,
    description: v.description,
    model: v.model,
    instructions: v.instructions,
    soul: v.soul,
    language: v.language,
    timezone: v.timezone,
    trigger: v.trigger,
    channels: v.channels,
    members: v.members ? v.members.split('\n').map((s) => s.trim()).filter(Boolean) : [],
    groups: v.groups ? v.groups.split('\n').map((s) => s.trim()).filter(Boolean) : [],
    tool_profile: v.tool_profile,
    tools_allow: v.tools_allow,
    tools_deny: v.tools_deny,
    max_turns: v.max_turns,
    run_timeout: v.run_timeout,
    skills: v.skills,
    identity: hasIdentity ? identity : undefined,
  }
}

function AgentDetailPanel({
  agent,
  onClose,
  onSaved,
  onDeleted,
  shared,
}: {
  agent: AgentInfo
  onClose: () => void
  onSaved: (updated: AgentInfo) => void
  onDeleted: (id: string) => void
  shared: SharedData
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<DetailTab>('overview')
  const [values, setValues] = useState<DetailValues>(() => agentToValues(agent))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [workspaceFiles, setWorkspaceFiles] = useState<{
    files: Record<string, string | null>
    inherited: Record<string, string>
  } | null>(null)
  const [pendingFileChanges, setPendingFileChanges] = useState<Record<string, string>>({})
  const [skillSearch, setSkillSearch] = useState('')

  useEffect(() => {
    if (agent.file_backed) {
      api.agents.files.list(agent.id).then(setWorkspaceFiles).catch(() => {})
    }
  }, [agent.id, agent.file_backed])

  const original = useMemo(() => agentToValues(agent), [agent])
  const hasChanges = useMemo(
    () => JSON.stringify(values) !== JSON.stringify(original),
    [values, original],
  )

  const resetValues = () => setValues(agentToValues(agent))

  const handleSave = useCallback(async (): Promise<boolean> => {
    setSaving(true)
    setError(null)
    try {
      await api.agents.update(agent.id, valuesToUpdate(values))
      const updated = await api.agents.get(agent.id)
      onSaved(updated)
      return true
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
      return false
    } finally {
      setSaving(false)
    }
  }, [agent.id, values, onSaved])

  const handleDelete = async () => {
    if (!window.confirm(t('agentsPage.deleteConfirm'))) return
    try {
      await api.agents.delete(agent.id)
      onDeleted(agent.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete')
    }
  }

  const set = <K extends keyof DetailValues>(key: K, val: DetailValues[K]) =>
    setValues((v) => ({ ...v, [key]: val }))

  const setIdentity = (key: keyof DetailValues['identity'], val: string) =>
    setValues((v) => ({ ...v, identity: { ...v.identity, [key]: val } }))

  const toggleChannel = (ch: string) =>
    set(
      'channels',
      values.channels.includes(ch)
        ? values.channels.filter((c) => c !== ch)
        : [...values.channels, ch],
    )

  const toggleSkill = (name: string) =>
    set(
      'skills',
      values.skills.includes(name)
        ? values.skills.filter((s) => s !== name)
        : [...values.skills, name],
    )

  // All known tool names from tool groups
  const allToolNames = useMemo(() => {
    const names: string[] = []
    for (const tools of Object.values(shared.toolGroups)) {
      names.push(...tools)
    }
    return [...new Set(names)].sort()
  }, [shared.toolGroups])

  // Active tool profile info
  const activeProfile = shared.toolProfiles.find((p) => p.name === values.tool_profile)

  // Skills filtered by search
  const filteredSkills = useMemo(() => {
    if (!skillSearch) return shared.skills
    const q = skillSearch.toLowerCase()
    return shared.skills.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.description.toLowerCase().includes(q),
    )
  }, [shared.skills, skillSearch])

  const tabs = [
    { id: 'overview' as const, label: 'Overview', icon: <Bot className="h-4 w-4" /> },
    { id: 'soul_identity' as const, label: 'Soul & Identity', icon: <Sparkles className="h-4 w-4" /> },
    { id: 'tools' as const, label: 'Tools', icon: <Wrench className="h-4 w-4" /> },
    { id: 'skills' as const, label: 'Skills', icon: <BookOpen className="h-4 w-4" /> },
    { id: 'routing' as const, label: 'Routing', icon: <Network className="h-4 w-4" /> },
    ...(agent.file_backed
      ? [{ id: 'files' as const, label: 'Files', icon: <FileText className="h-4 w-4" /> }]
      : []),
  ]

  return (
    <Card padding="lg" className="mt-6 border-brand/30">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-primary">
          {agent.identity?.emoji && <span className="mr-2">{agent.identity.emoji}</span>}
          {agent.name}
        </h3>
        <button onClick={onClose} className="text-tertiary hover:text-primary cursor-pointer">
          <X className="h-5 w-5" />
        </button>
      </div>

      <Tabs
        tabs={tabs}
        activeTab={tab}
        onChange={(id) => setTab(id as DetailTab)}
        className="mt-4"
      />

      <div className="mt-6">
        {/* ── Tab: Overview ── */}
        {tab === 'overview' && (
          <div className="space-y-5">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Name</label>
                <input
                  type="text"
                  value={values.name}
                  onChange={(e) => set('name', e.target.value)}
                  className={inputCls}
                />
              </div>
              <Select
                label="Model"
                value={values.model}
                onChange={(v) => set('model', v)}
                options={shared.modelOptions}
              />
              <div className="space-y-1.5 sm:col-span-2">
                <label className="text-sm font-medium text-primary">Description</label>
                <input
                  type="text"
                  value={values.description}
                  onChange={(e) => set('description', e.target.value)}
                  className={inputCls}
                />
              </div>
            </div>

            {/* Active + Default */}
            <div className="flex items-center gap-4">
              {agent.default ? (
                <Badge className="bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                  <Star className="mr-1 h-3 w-3" /> Default agent
                </Badge>
              ) : (
                <button
                  onClick={() => {
                    api.agents.setDefault(agent.id)
                      .then(() => api.agents.get(agent.id))
                      .then(onSaved)
                      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to set default'))
                  }}
                  className="cursor-pointer inline-flex items-center gap-1 text-sm text-secondary hover:text-amber-500 transition-colors"
                >
                  <Star className="h-4 w-4" /> Set as default
                </button>
              )}
            </div>

            {/* Instructions */}
            <div className="border-t border-secondary pt-5 space-y-1.5">
              <label className="text-sm font-medium text-primary">Instructions</label>
              <p className="text-xs text-tertiary">
                Operational guidelines — how this agent should behave, what it can/can't do
              </p>
              <textarea
                value={values.instructions}
                onChange={(e) => set('instructions', e.target.value)}
                rows={4}
                placeholder="Custom instructions for this agent..."
                className={textareaCls}
              />
            </div>

            {/* Quick stats */}
            <div className="border-t border-secondary pt-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
              <div className="rounded-lg border border-secondary p-3">
                <p className="text-xs text-tertiary">Sessions</p>
                <p className="text-lg font-semibold text-primary">{agent.session_count}</p>
              </div>
              <div className="rounded-lg border border-secondary p-3">
                <p className="text-xs text-tertiary">Members</p>
                <p className="text-lg font-semibold text-primary">{agent.member_count}</p>
              </div>
              <div className="rounded-lg border border-secondary p-3">
                <p className="text-xs text-tertiary">Groups</p>
                <p className="text-lg font-semibold text-primary">{agent.group_count}</p>
              </div>
              <div className="rounded-lg border border-secondary p-3">
                <p className="text-xs text-tertiary">File-backed</p>
                <p className="text-lg font-semibold text-primary">{agent.file_backed ? 'Yes' : 'No'}</p>
              </div>
            </div>

            {agent.workspace_dir && (
              <div className="space-y-1">
                <label className="text-xs font-medium text-tertiary">Workspace</label>
                <p className="text-xs text-tertiary font-mono bg-secondary px-2 py-1 rounded">
                  {agent.workspace_dir}
                </p>
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-2">
              <Select
                label="Language"
                value={values.language}
                onChange={(v) => set('language', v)}
                options={LANGUAGE_OPTIONS}
              />
              <Select
                label="Timezone"
                value={values.timezone}
                onChange={(v) => set('timezone', v)}
                options={TIMEZONE_OPTIONS}
              />
            </div>
          </div>
        )}

        {/* ── Tab: Soul & Identity ── */}
        {tab === 'soul_identity' && (
          <div className="space-y-6">
            {/* Soul section */}
            <div className="space-y-4">
              <div>
                <h4 className="text-sm font-semibold text-primary">Persona & Personality</h4>
                <p className="mt-1 text-xs text-tertiary">
                  Define who this agent is — personality traits, core truths, boundaries, communication style.
                </p>
                {agent.file_backed && (
                  <p className="text-xs text-tertiary mt-1 font-mono">
                    workspace-{agent.id}/SOUL.md — changes saved to file
                  </p>
                )}
              </div>
              <textarea
                value={values.soul}
                onChange={(e) => set('soul', e.target.value)}
                rows={12}
                placeholder="Define the agent's soul..."
                className={cn(textareaCls, 'font-mono text-xs')}
              />
              <Button
                variant="ghost"
                onClick={() => {
                  if (!values.soul.trim() || window.confirm('Replace current soul with template?')) {
                    set('soul', SOUL_TEMPLATE)
                  }
                }}
              >
                <Sparkles className="h-4 w-4" />
                Insert Template
              </Button>
            </div>

            {/* Identity section */}
            <div className="border-t border-secondary pt-6 space-y-5">
              <div>
                <h4 className="text-sm font-semibold text-primary">Identity</h4>
                <p className="mt-1 text-xs text-tertiary">
                  Visual identity and personality cues for this agent.
                </p>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-primary">Name</label>
                  <input
                    type="text"
                    value={values.identity.name}
                    onChange={(e) => setIdentity('name', e.target.value)}
                    placeholder='e.g. "Aria"'
                    className={inputCls}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-primary">Emoji</label>
                  <input
                    type="text"
                    value={values.identity.emoji}
                    onChange={(e) => setIdentity('emoji', e.target.value)}
                    placeholder='e.g. "🦊"'
                    className={inputCls}
                  />
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-primary">Creature</label>
                  <input
                    type="text"
                    value={values.identity.creature}
                    onChange={(e) => setIdentity('creature', e.target.value)}
                    placeholder='e.g. "fox", "owl", "ghost in the machine"'
                    className={inputCls}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium text-primary">Vibe</label>
                  <input
                    type="text"
                    value={values.identity.vibe}
                    onChange={(e) => setIdentity('vibe', e.target.value)}
                    placeholder='e.g. "chill mentor", "sharp & direct"'
                    className={inputCls}
                  />
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Theme</label>
                <input
                  type="text"
                  value={values.identity.theme}
                  onChange={(e) => setIdentity('theme', e.target.value)}
                  placeholder='e.g. "helpful hacker"'
                  className={inputCls}
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Avatar</label>
                <p className="text-xs text-tertiary">URL, data URI, or workspace-relative path</p>
                <input
                  type="text"
                  value={values.identity.avatar}
                  onChange={(e) => setIdentity('avatar', e.target.value)}
                  placeholder="https://..."
                  className={inputCls}
                />
              </div>
            </div>
          </div>
        )}

        {/* ── Tab: Tools ── */}
        {tab === 'tools' && (
          <div className="space-y-6">
            {/* Tool Profile Selector */}
            <div className="space-y-2">
              <h4 className="text-sm font-semibold text-primary">Tool Profile</h4>
              <p className="text-xs text-tertiary">
                Profiles control which tools are available to this agent
              </p>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={() => set('tool_profile', '')}
                  className={cn(
                    'rounded-lg border px-4 py-2 text-sm font-medium transition-colors cursor-pointer',
                    !values.tool_profile
                      ? 'border-brand bg-brand-secondary text-brand-tertiary'
                      : 'border-secondary text-secondary hover:border-primary hover:text-primary',
                  )}
                >
                  Default
                </button>
                {shared.toolProfiles.map((p) => (
                  <button
                    key={p.name}
                    type="button"
                    onClick={() => set('tool_profile', p.name)}
                    className={cn(
                      'rounded-lg border px-4 py-2 text-sm font-medium transition-colors cursor-pointer',
                      values.tool_profile === p.name
                        ? 'border-brand bg-brand-secondary text-brand-tertiary'
                        : 'border-secondary text-secondary hover:border-primary hover:text-primary',
                    )}
                    title={p.description}
                  >
                    {p.name}
                    {p.builtin && (
                      <span className="ml-1.5 text-[10px] text-quaternary">(built-in)</span>
                    )}
                  </button>
                ))}
              </div>

              {/* Active profile details */}
              {activeProfile && (
                <div className="mt-3 rounded-lg border border-secondary bg-secondary/30 p-3">
                  <p className="text-xs font-medium text-primary">{activeProfile.name}</p>
                  {activeProfile.description && (
                    <p className="mt-0.5 text-xs text-tertiary">{activeProfile.description}</p>
                  )}
                  {activeProfile.allow.length > 0 && (
                    <p className="mt-1.5 text-xs text-tertiary">
                      <span className="font-medium text-fg-success-secondary">Allow:</span>{' '}
                      {activeProfile.allow.join(', ')}
                    </p>
                  )}
                  {activeProfile.deny.length > 0 && (
                    <p className="mt-0.5 text-xs text-tertiary">
                      <span className="font-medium text-fg-error-secondary">Deny:</span>{' '}
                      {activeProfile.deny.join(', ')}
                    </p>
                  )}
                </div>
              )}
            </div>

            {/* Tool Groups (collapsible) */}
            {Object.keys(shared.toolGroups).length > 0 && (
              <details className="rounded-lg border border-secondary">
                <summary className="cursor-pointer px-3 py-2.5 text-sm font-medium text-secondary hover:text-primary">
                  Tool Groups ({Object.keys(shared.toolGroups).length} categories)
                </summary>
                <div className="grid gap-2 px-3 pb-3 sm:grid-cols-2 lg:grid-cols-3">
                  {Object.entries(shared.toolGroups).map(([group, tools]) => (
                    <div
                      key={group}
                      className="rounded-lg bg-secondary/50 p-2.5"
                    >
                      <p className="text-xs font-semibold text-primary">{group}</p>
                      <p className="mt-0.5 text-[10px] text-quaternary">
                        {tools.join(', ')}
                      </p>
                    </div>
                  ))}
                </div>
              </details>
            )}

            {/* Allow/Deny overrides */}
            <div className="space-y-4">
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Allow Overrides</label>
                <p className="text-xs text-tertiary">
                  Individual tools to allow beyond the profile
                </p>
                <ChipInput
                  value={values.tools_allow}
                  onChange={(v) => set('tools_allow', v)}
                  placeholder="Type tool name and press Enter"
                  suggestions={allToolNames}
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Deny Overrides</label>
                <p className="text-xs text-tertiary">
                  Individual tools to deny from the profile
                </p>
                <ChipInput
                  value={values.tools_deny}
                  onChange={(v) => set('tools_deny', v)}
                  placeholder="Type tool name and press Enter"
                  suggestions={allToolNames}
                />
              </div>
            </div>

            {/* Max turns / timeout */}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Max Turns</label>
                <input
                  type="number"
                  value={values.max_turns}
                  onChange={(e) => set('max_turns', parseInt(e.target.value) || 0)}
                  min={0}
                  placeholder="0 = unlimited"
                  className={inputCls}
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium text-primary">Run Timeout (s)</label>
                <input
                  type="number"
                  value={values.run_timeout}
                  onChange={(e) => set('run_timeout', parseInt(e.target.value) || 0)}
                  min={0}
                  placeholder="0 = unlimited"
                  className={inputCls}
                />
              </div>
            </div>
          </div>
        )}

        {/* ── Tab: Skills ── */}
        {tab === 'skills' && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-sm font-semibold text-primary flex items-center gap-2">
                  Skills
                  {values.skills.length > 0 && (
                    <Badge className="bg-brand-secondary text-brand-tertiary text-[10px] px-1.5 py-0">
                      {values.skills.length}
                    </Badge>
                  )}
                </h4>
                <p className="mt-0.5 text-xs text-tertiary">
                  Enable or disable skills for this agent
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => set('skills', shared.skills.filter((s) => s.enabled).map((s) => s.name))}
                >
                  Enable All
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => set('skills', [])}
                >
                  Disable All
                </Button>
              </div>
            </div>

            {/* Search */}
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-quaternary" />
              <input
                type="text"
                value={skillSearch}
                onChange={(e) => setSkillSearch(e.target.value)}
                placeholder="Filter skills..."
                className={cn(inputCls, 'pl-9')}
              />
            </div>

            {/* Skill list */}
            {!shared.loaded ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-5 w-5 animate-spin text-tertiary" />
              </div>
            ) : filteredSkills.length === 0 ? (
              <p className="text-sm text-tertiary text-center py-4">
                {skillSearch ? 'No skills match your search' : 'No skills available'}
              </p>
            ) : (
              <div className="space-y-1 max-h-[400px] overflow-y-auto">
                {filteredSkills.map((skill) => (
                  <div
                    key={skill.name}
                    className={cn(
                      'flex items-center justify-between rounded-lg border px-3 py-2.5 transition-colors',
                      values.skills.includes(skill.name)
                        ? 'border-brand/30 bg-brand-secondary/30'
                        : 'border-secondary',
                    )}
                  >
                    <div className="flex-1 min-w-0 mr-3">
                      <p className="text-sm font-medium text-primary truncate">{skill.name}</p>
                      {skill.description && (
                        <p className="text-xs text-tertiary truncate">{skill.description}</p>
                      )}
                    </div>
                    <Toggle
                      checked={values.skills.includes(skill.name)}
                      onChange={() => toggleSkill(skill.name)}
                      size="sm"
                    />
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Tab: Routing ── */}
        {tab === 'routing' && (
          <div className="space-y-6">
            {/* Channel Routing */}
            <div className="space-y-2">
              <h4 className="text-sm font-semibold text-primary">Channels</h4>
              <p className="text-xs text-tertiary">
                Route all messages from selected channels to this agent.
              </p>
              {shared.channels.length > 0 ? (
                <div className="grid gap-2 sm:grid-cols-2">
                  {shared.channels.map((ch) => {
                    const chId = ch.full_id || ch.name
                    const label = ch.account_id ? `${ch.name} (${ch.account_id})` : ch.name
                    const selected = values.channels.includes(chId)
                    return (
                      <button
                        key={chId}
                        type="button"
                        onClick={() => toggleChannel(chId)}
                        className={cn(
                          'flex items-center gap-3 rounded-lg border p-3 text-left transition-colors cursor-pointer',
                          selected
                            ? 'border-brand bg-brand-secondary/30'
                            : 'border-secondary hover:border-primary',
                        )}
                      >
                        <StatusDot
                          status={ch.connected ? 'online' : ch.configured ? 'offline' : 'warning'}
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium text-primary">{label}</p>
                          <p className="text-xs text-tertiary">
                            {ch.connected ? 'Connected' : ch.configured ? 'Configured' : 'Not configured'}
                          </p>
                        </div>
                        {selected && (
                          <CheckCircle2 className="h-4 w-4 text-brand-tertiary shrink-0" />
                        )}
                      </button>
                    )
                  })}
                </div>
              ) : (
                <p className="text-xs text-quaternary">No channels configured</p>
              )}
            </div>

            {/* Members */}
            <div className="space-y-1.5">
              <h4 className="text-sm font-semibold text-primary">Members</h4>
              <p className="text-xs text-tertiary">User IDs routed to this agent (one per line)</p>
              <textarea
                value={values.members}
                onChange={(e) => set('members', e.target.value)}
                rows={3}
                placeholder={"user@example.com\n5521999999999@s.whatsapp.net\ntelegram:123456"}
                className={textareaCls}
              />
            </div>

            {/* Groups */}
            <div className="space-y-1.5">
              <h4 className="text-sm font-semibold text-primary">Groups</h4>
              <p className="text-xs text-tertiary">Group IDs routed to this agent (one per line)</p>
              <textarea
                value={values.groups}
                onChange={(e) => set('groups', e.target.value)}
                rows={3}
                placeholder={"120363001234567890@g.us\ntelegram:-1001234567890"}
                className={textareaCls}
              />
            </div>

            {/* Trigger */}
            <div className="space-y-1.5">
              <label className="text-sm font-medium text-primary">Trigger</label>
              <p className="text-xs text-tertiary">Override the activation keyword for this agent</p>
              <input
                type="text"
                value={values.trigger}
                onChange={(e) => set('trigger', e.target.value)}
                placeholder="Global default"
                className={inputCls}
              />
            </div>
          </div>
        )}

        {/* ── Tab: Files ── */}
        {tab === 'files' && agent.file_backed && (
          <div className="space-y-3">
            <div>
              <label className="text-xs font-semibold text-primary">Workspace Directory</label>
              <p className="mt-1 text-xs text-tertiary font-mono bg-secondary px-2 py-1 rounded">
                {agent.workspace_dir}
              </p>
            </div>

            {BOOTSTRAP_FILES.map((filename) => {
              const content = workspaceFiles?.files?.[filename]
              const inherited = workspaceFiles?.inherited?.[filename]
              const isOwn = content != null
              const isInherited = inherited != null
              const pendingContent = pendingFileChanges[filename]

              return (
                <div key={filename} className="border border-secondary rounded-lg p-3">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-sm font-medium text-primary">{filename}</span>
                    <div className="flex items-center gap-2">
                      {isOwn && pendingContent !== undefined && pendingContent !== (content ?? '') && (
                        <button
                          type="button"
                          onClick={() =>
                            setPendingFileChanges((prev) => {
                              const next = { ...prev }
                              delete next[filename]
                              return next
                            })
                          }
                          className="text-xs text-tertiary hover:text-primary cursor-pointer"
                        >
                          Reset
                        </button>
                      )}
                      {isOwn && <Badge variant="success">Own</Badge>}
                      {isInherited && !isOwn && <Badge>Inherited</Badge>}
                      {!isOwn && !isInherited && <Badge variant="warning">Not set</Badge>}
                    </div>
                  </div>

                  {isOwn && (
                    <textarea
                      className={cn(textareaCls, 'font-mono text-xs')}
                      rows={6}
                      value={pendingContent ?? content ?? ''}
                      onChange={(e) =>
                        setPendingFileChanges((prev) => ({ ...prev, [filename]: e.target.value }))
                      }
                    />
                  )}
                  {isInherited && !isOwn && (
                    <p className="text-xs text-tertiary">
                      Using global: <code className="bg-secondary px-1 rounded">{inherited}</code>
                      <button
                        type="button"
                        onClick={async () => {
                          try {
                            await api.agents.files.update(agent.id, filename, '')
                            const updated = await api.agents.files.list(agent.id)
                            setWorkspaceFiles(updated)
                          } catch {
                            setError('Failed to override file')
                          }
                        }}
                        className="ml-2 text-brand-tertiary hover:underline cursor-pointer"
                      >
                        Override
                      </button>
                    </p>
                  )}
                  {!isOwn && !isInherited && (
                    <button
                      type="button"
                      onClick={async () => {
                        try {
                          await api.agents.files.update(agent.id, filename, `# ${filename.replace('.md', '')}\n`)
                          const updated = await api.agents.files.list(agent.id)
                          setWorkspaceFiles(updated)
                        } catch {
                          setError('Failed to create file')
                        }
                      }}
                      className="text-xs text-brand-tertiary hover:underline cursor-pointer"
                    >
                      Create
                    </button>
                  )}
                </div>
              )
            })}

            {Object.keys(pendingFileChanges).length > 0 && (
              <Button
                size="sm"
                onClick={async () => {
                  const entries = Object.entries(pendingFileChanges)
                  const results = await Promise.allSettled(
                    entries.map(([filename, content]) =>
                      api.agents.files.update(agent.id, filename, content).then(() => filename),
                    ),
                  )
                  const failed = results
                    .filter((r): r is PromiseRejectedResult => r.status === 'rejected')
                  const succeeded = results
                    .filter((r): r is PromiseFulfilledResult<string> => r.status === 'fulfilled')
                    .map((r) => r.value)

                  // Clear only the files that saved successfully
                  if (succeeded.length > 0) {
                    setPendingFileChanges((prev) => {
                      const next = { ...prev }
                      for (const f of succeeded) delete next[f]
                      return next
                    })
                    const updated = await api.agents.files.list(agent.id).catch(() => null)
                    if (updated) setWorkspaceFiles(updated)
                  }
                  if (failed.length > 0) {
                    setError(`Failed to save ${failed.length} file(s)`)
                  }
                }}
              >
                <Save className="h-4 w-4" />
                Save File Changes
              </Button>
            )}
          </div>
        )}
      </div>

      {error && <p className="mt-4 text-sm text-fg-error-secondary">{error}</p>}

      <div className="mt-6 flex items-center gap-3">
        <div className="flex-1" />
        <Button variant="destructive-subtle" onClick={handleDelete}>
          <Trash2 className="h-4 w-4" />
          Delete
        </Button>
      </div>

      {/* Unsaved changes bar */}
      <UnsavedChangesBar
        hasChanges={hasChanges}
        saving={saving}
        onSave={handleSave}
        onDiscard={resetValues}
      />
    </Card>
  )
}
