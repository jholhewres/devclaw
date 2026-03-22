import { useCallback, useEffect, useState } from 'react'
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
} from 'lucide-react'
import { api, type AgentInfo, type CreateAgentRequest, type UpdateAgentRequest } from '@/lib/api'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { SearchInput } from '@/components/ui/SearchInput'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'
import { LoadingSpinner, ErrorState } from '@/components/ui/ConfigComponents'

export function Agents() {
  const { t } = useTranslation()
  const [agents, setAgents] = useState<AgentInfo[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)

  useEffect(() => {
    api.agents
      .list()
      .then(setAgents)
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false))
  }, [])

  // Auto-clear action errors after 3s
  useEffect(() => {
    if (!actionError) return
    const timer = setTimeout(() => setActionError(null), 3000)
    return () => clearTimeout(timer)
  }, [actionError])

  const filtered = agents.filter(
    (a) =>
      a.name.toLowerCase().includes(search.toLowerCase()) ||
      a.description?.toLowerCase().includes(search.toLowerCase()) ||
      a.id.toLowerCase().includes(search.toLowerCase()),
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
    if (!window.confirm('Delete this agent? All its sessions will be removed.')) return
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
          title="Agents"
          description={`${activeCount} active / ${agents.length}`}
        />
        <Button
          onClick={() => setShowCreate(!showCreate)}
          variant={showCreate ? 'ghost' : 'default'}
        >
          {showCreate ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
          {showCreate ? t('common.cancel') : 'New Agent'}
        </Button>
      </div>

      {showCreate && (
        <CreateAgentPanel
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      )}

      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder="Search agents..."
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
                    default
                  </Badge>
                )}
                {isPlugin && (
                  <Badge className="bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400">
                    plugin
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
              <h3 className="mt-4 text-lg font-semibold text-primary">{agent.name}</h3>
              {agent.model && (
                <p className="mt-0.5 text-xs text-tertiary">{agent.model}</p>
              )}
              {agent.description && (
                <p className="mt-2 text-sm leading-relaxed text-secondary line-clamp-2">
                  {agent.description}
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
          title={search ? 'No agents match your search' : 'No agents configured'}
          description={!search ? 'Create an agent to customize assistant behavior per channel or user.' : undefined}
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
        />
      )}
    </div>
  )
}

// ── Create Agent Panel ──

function CreateAgentPanel({
  onCreated,
  onCancel,
}: {
  onCreated: (agent: AgentInfo) => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [model, setModel] = useState('')
  const [emoji, setEmoji] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = async () => {
    if (!name.trim()) return
    setCreating(true)
    setError(null)
    try {
      const req: CreateAgentRequest = {
        name: name.trim(),
        description: description.trim() || undefined,
        model: model.trim() || undefined,
        identity: emoji.trim() ? { emoji: emoji.trim() } : undefined,
      }
      const result = await api.agents.create(req)
      // Fetch the created agent by its returned ID
      const created = await api.agents.get(result.id)
      onCreated(created)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create agent')
    } finally {
      setCreating(false)
    }
  }

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
            className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            disabled={creating}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Emoji</label>
          <input
            type="text"
            value={emoji}
            onChange={(e) => setEmoji(e.target.value)}
            placeholder="e.g. robot_face"
            className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            disabled={creating}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Model</label>
          <input
            type="text"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="e.g. claude-sonnet-4-20250514"
            className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            disabled={creating}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Description</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Short description"
            className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            disabled={creating}
          />
        </div>
      </div>
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

// ── Agent Detail Panel ──

function AgentDetailPanel({
  agent,
  onClose,
  onSaved,
  onDeleted,
}: {
  agent: AgentInfo
  onClose: () => void
  onSaved: (updated: AgentInfo) => void
  onDeleted: (id: string) => void
}) {
  const { t } = useTranslation()
  const [values, setValues] = useState<UpdateAgentRequest>({
    name: agent.name,
    description: agent.description ?? '',
    model: agent.model ?? '',
    instructions: agent.instructions ?? '',
    language: agent.language ?? '',
    channels: agent.channels ?? [],
    tool_profile: agent.tool_profile ?? '',
    max_turns: agent.max_turns ?? 0,
    run_timeout: agent.run_timeout ?? 0,
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Track if there are unsaved changes — compare against initial agent state
  const makeOriginal = () => JSON.stringify({
    name: agent.name,
    description: agent.description ?? '',
    model: agent.model ?? '',
    instructions: agent.instructions ?? '',
    language: agent.language ?? '',
    channels: agent.channels ?? [],
    tool_profile: agent.tool_profile ?? '',
    max_turns: agent.max_turns ?? 0,
    run_timeout: agent.run_timeout ?? 0,
  })
  const hasChanges = JSON.stringify(values) !== makeOriginal()

  const resetValues = () => {
    setValues({
      name: agent.name,
      description: agent.description ?? '',
      model: agent.model ?? '',
      instructions: agent.instructions ?? '',
      language: agent.language ?? '',
      channels: agent.channels ?? [],
      tool_profile: agent.tool_profile ?? '',
      max_turns: agent.max_turns ?? 0,
      run_timeout: agent.run_timeout ?? 0,
    })
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await api.agents.update(agent.id, values)
      const updated = await api.agents.get(agent.id)
      onSaved(updated)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!window.confirm('Delete this agent? All its sessions will be removed.')) return
    try {
      await api.agents.delete(agent.id)
      onDeleted(agent.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete')
    }
  }

  const channelsStr = (values.channels ?? []).join(', ')

  return (
    <Card padding="lg" className="mt-6 border-brand/30">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-primary">
          {agent.name} — Settings
        </h3>
        <button onClick={onClose} className="text-tertiary hover:text-primary cursor-pointer">
          <X className="h-5 w-5" />
        </button>
      </div>

      <div className="mt-6 space-y-5">
        {/* Overview */}
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Name</label>
            <input
              type="text"
              value={values.name ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, name: e.target.value }))}
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Model</label>
            <input
              type="text"
              value={values.model ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, model: e.target.value }))}
              placeholder="Global default"
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <label className="text-sm font-medium text-primary">Description</label>
            <input
              type="text"
              value={values.description ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, description: e.target.value }))}
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
        </div>

        {/* Instructions */}
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-primary">Instructions</label>
          <p className="text-xs text-tertiary">System prompt for this agent</p>
          <textarea
            value={values.instructions ?? ''}
            onChange={(e) => setValues((v) => ({ ...v, instructions: e.target.value }))}
            rows={6}
            placeholder="Custom instructions for this agent..."
            className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid resize-y"
          />
        </div>

        {/* Routing */}
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Channels</label>
            <p className="text-xs text-tertiary">Comma-separated (e.g. slack, telegram)</p>
            <input
              type="text"
              value={channelsStr}
              onChange={(e) =>
                setValues((v) => ({
                  ...v,
                  channels: e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean),
                }))
              }
              placeholder="All channels"
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Tool Profile</label>
            <select
              value={values.tool_profile ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, tool_profile: e.target.value }))}
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            >
              <option value="">Default</option>
              <option value="minimal">Minimal</option>
              <option value="coding">Coding</option>
              <option value="messaging">Messaging</option>
              <option value="full">Full</option>
            </select>
          </div>
        </div>

        {/* Advanced */}
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Max Turns</label>
            <input
              type="number"
              value={values.max_turns ?? 0}
              onChange={(e) => setValues((v) => ({ ...v, max_turns: parseInt(e.target.value) || 0 }))}
              min={0}
              placeholder="0 = unlimited"
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-primary">Run Timeout (s)</label>
            <input
              type="number"
              value={values.run_timeout ?? 0}
              onChange={(e) => setValues((v) => ({ ...v, run_timeout: parseInt(e.target.value) || 0 }))}
              min={0}
              placeholder="0 = unlimited"
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <label className="text-sm font-medium text-primary">Language</label>
            <input
              type="text"
              value={values.language ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, language: e.target.value }))}
              placeholder="Global default"
              className="w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-primary placeholder:text-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid"
            />
          </div>
        </div>
      </div>

      {error && <p className="mt-4 text-sm text-fg-error-secondary">{error}</p>}

      <div className="mt-6 flex items-center gap-3">
        {hasChanges && (
          <>
            <Button onClick={handleSave} disabled={saving}>
              <Save className="h-4 w-4" />
              {saving ? t('common.saving') : t('common.save')}
            </Button>
            <Button variant="ghost" onClick={resetValues}>
              Discard
            </Button>
          </>
        )}
        <div className="flex-1" />
        <Button variant="destructive-subtle" onClick={handleDelete}>
          <Trash2 className="h-4 w-4" />
          Delete
        </Button>
      </div>
    </Card>
  )
}
