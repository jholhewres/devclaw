import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Workflow,
  Plus,
  X,
  Play,
  Pause,
  Trash2,
  ChevronRight,
  Loader2,
  Clock,
  CheckCircle2,
  Circle,
  Search,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { SearchInput } from '@/components/ui/SearchInput'
import { Badge } from '@/components/ui/Badge'
import { Card } from '@/components/ui/Card'
import { EmptyState } from '@/components/ui/EmptyState'
import { Button } from '@/components/ui/Button'

// ── Types ──

type WorkflowStatus = 'active' | 'draft' | 'paused' | 'completed'

interface WorkflowStep {
  id: string
  name: string
  agent: string
  status?: 'pending' | 'running' | 'done' | 'failed'
}

interface WorkflowData {
  id: string
  name: string
  description: string
  status: WorkflowStatus
  steps: WorkflowStep[]
  lastRun: string | null
  createdAt: string
}

// ── Mock data ──

const MOCK_AGENTS = ['Default Agent', 'Support Bot', 'Sales Assistant', 'Code Reviewer', 'Data Analyst']

const MOCK_WORKFLOWS: WorkflowData[] = [
  {
    id: 'wf-1',
    name: 'Lead Qualification Pipeline',
    description: 'Qualify incoming leads through analysis, scoring, and outreach stages',
    status: 'active',
    steps: [
      { id: 's1', name: 'Analyze Lead', agent: 'Data Analyst', status: 'done' },
      { id: 's2', name: 'Score & Classify', agent: 'Default Agent', status: 'done' },
      { id: 's3', name: 'Draft Outreach', agent: 'Sales Assistant', status: 'running' },
    ],
    lastRun: '2 hours ago',
    createdAt: '2026-06-01',
  },
  {
    id: 'wf-2',
    name: 'Code Review Flow',
    description: 'Automated code review with security check and documentation',
    status: 'draft',
    steps: [
      { id: 's1', name: 'Review Code', agent: 'Code Reviewer', status: 'pending' },
      { id: 's2', name: 'Security Scan', agent: 'Default Agent', status: 'pending' },
      { id: 's3', name: 'Generate Docs', agent: 'Default Agent', status: 'pending' },
    ],
    lastRun: null,
    createdAt: '2026-06-03',
  },
  {
    id: 'wf-3',
    name: 'Customer Onboarding',
    description: 'Step-by-step onboarding with personalized welcome and setup guidance',
    status: 'paused',
    steps: [
      { id: 's1', name: 'Welcome Message', agent: 'Support Bot', status: 'done' },
      { id: 's2', name: 'Setup Guide', agent: 'Support Bot', status: 'pending' },
    ],
    lastRun: '1 day ago',
    createdAt: '2026-05-28',
  },
]

// ── Helpers ──

const STATUS_META: Record<WorkflowStatus, { color: string; labelKey: string }> = {
  active: { color: 'bg-fg-success-secondary', labelKey: 'workflows.status.active' },
  draft: { color: 'bg-fg-quaternary', labelKey: 'workflows.status.draft' },
  paused: { color: 'bg-fg-warning-secondary', labelKey: 'workflows.status.paused' },
  completed: { color: 'bg-brand-blue-400', labelKey: 'workflows.status.completed' },
}

const STEP_ICON = {
  done: <CheckCircle2 className="size-3.5 text-fg-success-secondary" />,
  running: <Loader2 className="size-3.5 animate-spin text-brand-blue-400" />,
  failed: <X className="size-3.5 text-fg-error-secondary" />,
  pending: <Circle className="size-3.5 text-fg-quaternary" />,
}

// ── Main Component ──

export function Workflows() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)

  const filtered = MOCK_WORKFLOWS.filter(
    (wf) =>
      wf.name.toLowerCase().includes(search.toLowerCase()) ||
      wf.description.toLowerCase().includes(search.toLowerCase()),
  )

  const activeCount = MOCK_WORKFLOWS.filter((wf) => wf.status === 'active').length

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8">
      <div className="flex items-center justify-between">
        <PageHeader
          title={t('workflows.title')}
          description={`${activeCount} ${t('workflows.active').toLowerCase()} / ${MOCK_WORKFLOWS.length}`}
        />
        <Button onClick={() => setShowCreate(!showCreate)}>
          {showCreate ? <X className="size-4" /> : <Plus className="size-4" />}
          {showCreate ? t('common.cancel') : t('workflows.create')}
        </Button>
      </div>

      {showCreate && <CreateWorkflowPanel onClose={() => setShowCreate(false)} />}

      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder={t('workflows.searchPlaceholder')}
        className="mt-6"
      />

      <div className="mt-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {filtered.map((wf) => (
          <WorkflowCard key={wf.id} workflow={wf} />
        ))}
      </div>

      {filtered.length === 0 && (
        <EmptyState
          icon={<Workflow className="size-6" />}
          title={search ? t('workflows.emptySearch') : t('workflows.empty')}
          description={!search ? t('workflows.emptyDesc') : undefined}
        />
      )}
    </div>
  )
}

// ── Workflow Card ──

function WorkflowCard({ workflow }: { workflow: WorkflowData }) {
  const { t } = useTranslation()
  const statusMeta = STATUS_META[workflow.status]

  return (
    <Card
      padding="lg"
      className={cn(
        'group relative overflow-hidden transition-all',
        workflow.status === 'active'
          ? 'border-brand/30 hover:border-brand/50'
          : 'hover:border-primary',
      )}
    >
      {/* Status badge */}
      <div className="absolute right-4 top-4">
        <Badge className={cn('gap-1.5', statusMeta.color)}>
          <span className="inline-block size-1.5 rounded-full bg-current" />
          {t(statusMeta.labelKey)}
        </Badge>
      </div>

      {/* Icon */}
      <div
        className={cn(
          'flex size-12 items-center justify-center rounded-xl transition-colors',
          workflow.status === 'active'
            ? 'bg-brand-secondary text-brand-tertiary'
            : 'bg-secondary text-fg-tertiary group-hover:text-fg-secondary',
        )}
      >
        <Workflow className="size-6" />
      </div>

      {/* Info */}
      <h3 className="mt-4 text-lg font-semibold text-fg-primary">{workflow.name}</h3>
      <p className="mt-1 text-sm leading-relaxed text-fg-secondary line-clamp-2">
        {workflow.description}
      </p>

      {/* Steps stepper */}
      <div className="mt-4 flex items-center gap-1 overflow-x-auto pb-1">
        {workflow.steps.map((step, i) => (
          <div key={step.id} className="flex items-center gap-1 shrink-0">
            <div
              className={cn(
                'flex items-center gap-1.5 rounded-lg border px-2 py-1 text-xs font-medium transition-colors',
                step.status === 'done'
                  ? 'border-success/30 bg-success/10 text-fg-success-secondary'
                  : step.status === 'running'
                    ? 'border-brand-blue-400/30 bg-brand-blue-400/10 text-brand-blue-400'
                    : 'border-secondary text-fg-tertiary',
              )}
            >
              {STEP_ICON[step.status ?? 'pending']}
              <span className="max-w-[80px] truncate">{step.name}</span>
            </div>
            {i < workflow.steps.length - 1 && (
              <ChevronRight className="size-3 shrink-0 text-fg-quaternary" />
            )}
          </div>
        ))}
      </div>

      {/* Footer */}
      <div className="mt-4 flex items-center justify-between text-xs text-fg-tertiary">
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-1">
            {workflow.steps.length} {t('workflows.steps').toLowerCase()}
          </span>
          {workflow.lastRun && (
            <span className="flex items-center gap-1">
              <Clock className="size-3" />
              {workflow.lastRun}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <button className="cursor-pointer rounded-md p-1 text-fg-tertiary hover:bg-primary_hover hover:text-fg-primary">
            {workflow.status === 'active' ? <Pause className="size-4" /> : <Play className="size-4" />}
          </button>
          <button className="cursor-pointer rounded-md p-1 text-fg-tertiary hover:bg-primary_hover hover:text-fg-error-secondary">
            <Trash2 className="size-4" />
          </button>
        </div>
      </div>
    </Card>
  )
}

// ── Create Workflow Panel ──

function CreateWorkflowPanel({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [steps, setSteps] = useState<Partial<WorkflowStep>[]>([
    { id: 's1', name: '', agent: '' },
  ])

  const addStep = () => {
    setSteps([...steps, { id: `s${steps.length + 1}`, name: '', agent: '' }])
  }

  const removeStep = (idx: number) => {
    if (steps.length <= 1) return
    setSteps(steps.filter((_, i) => i !== idx))
  }

  const updateStep = (idx: number, field: 'name' | 'agent', value: string) => {
    const updated = [...steps]
    updated[idx] = { ...updated[idx], [field]: value }
    setSteps(updated)
  }

  const inputCls =
    'w-full rounded-lg border border-secondary bg-primary px-3 py-2 text-sm text-fg-primary placeholder:text-fg-quaternary focus:border-brand-solid focus:ring-1 focus:ring-brand-solid'

  return (
    <Card padding="lg" className="mt-4 border-brand/30">
      <h3 className="text-sm font-semibold text-fg-primary">{t('workflows.create')}</h3>
      <p className="mt-1 text-xs text-fg-tertiary">{t('workflows.createDesc')}</p>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-fg-primary">{t('workflows.nameLabel')}</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('workflows.namePlaceholder')}
            className={inputCls}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium text-fg-primary">{t('workflows.descriptionLabel')}</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('workflows.descriptionPlaceholder')}
            className={inputCls}
          />
        </div>
      </div>

      {/* Steps builder */}
      <div className="mt-6 space-y-3">
        <div className="flex items-center justify-between">
          <label className="text-sm font-semibold text-fg-primary">{t('workflows.steps')}</label>
          <Button size="sm" variant="ghost" onClick={addStep}>
            <Plus className="size-3.5" />
            {t('workflows.addStep')}
          </Button>
        </div>

        {steps.map((step, idx) => (
          <div key={step.id} className="flex items-center gap-2">
            <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary text-xs font-bold text-fg-tertiary">
              {idx + 1}
            </div>
            <input
              type="text"
              value={step.name ?? ''}
              onChange={(e) => updateStep(idx, 'name', e.target.value)}
              placeholder={t('workflows.stepNamePlaceholder')}
              className={cn(inputCls, 'flex-1')}
            />
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-quaternary" />
              <select
                value={step.agent ?? ''}
                onChange={(e) => updateStep(idx, 'agent', e.target.value)}
                className={cn(inputCls, 'pl-8 pr-8 appearance-none min-w-[140px]')}
              >
                <option value="">{t('workflows.selectAgent')}</option>
                {MOCK_AGENTS.map((agent) => (
                  <option key={agent} value={agent}>
                    {agent}
                  </option>
                ))}
              </select>
            </div>
            <button
              onClick={() => removeStep(idx)}
              disabled={steps.length <= 1}
              className="cursor-pointer shrink-0 rounded-md p-1.5 text-fg-tertiary transition-colors hover:bg-primary_hover hover:text-fg-error-secondary disabled:opacity-30 disabled:cursor-not-allowed"
            >
              <Trash2 className="size-4" />
            </button>
          </div>
        ))}
      </div>

      <div className="mt-6 flex items-center gap-2">
        <Button disabled={!name.trim()}>
          {t('workflows.create')}
        </Button>
        <Button variant="ghost" onClick={onClose}>
          {t('common.cancel')}
        </Button>
      </div>
    </Card>
  )
}
