import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Clock, Play, Pause, AlertCircle, Timer, Wrench, CalendarClock, FileCode } from 'lucide-react'
import { api, type JobInfo } from '@/lib/api'
import { timeAgo, cn } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { EmptyState } from '@/components/ui/EmptyState'
import { LoadingSpinner } from '@/components/ui/ConfigComponents'

export function Jobs() {
  const { t } = useTranslation()
  const [jobs, setJobs] = useState<JobInfo[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.jobs.list()
      .then(setJobs)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return <LoadingSpinner />
  }

  const active = jobs.filter((j) => j.enabled)
  const inactive = jobs.filter((j) => !j.enabled)

  return (
    <div className="py-8 px-4 sm:px-6 lg:px-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <PageHeader
        title={t('jobs.title')}
        description={t('jobs.subtitle')}
      />

      {jobs.length === 0 ? (
        <EmptyJobs />
      ) : (
        <div className="mt-6 space-y-6">
          {active.length > 0 && (
            <div className="space-y-2">
              {active.map((job) => <JobCard key={job.id} job={job} />)}
            </div>
          )}
          {inactive.length > 0 && (
            <div>
              {active.length > 0 && (
                <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-text-muted">{t('common.disabled')}</p>
              )}
              <div className="space-y-2">
                {inactive.map((job) => <JobCard key={job.id} job={job} />)}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function EmptyJobs() {
  const { t } = useTranslation()
  return (
    <Card padding="lg" className="mt-8 rounded-2xl">
      <EmptyState
        icon={<CalendarClock className="h-6 w-6" />}
        title={t('jobs.noJobs')}
        description={t('jobs.noJobsDescription')}
      />

      <div className="mt-6 mx-auto max-w-md rounded-xl bg-bg-main px-4 py-3 border border-border">
        <p className="text-[11px] font-semibold uppercase tracking-wider text-text-muted">config.yaml</p>
        <pre className="mt-2 overflow-x-auto font-mono text-xs leading-relaxed text-text-secondary">
{`scheduler:
  jobs:
    - id: backup-daily
      schedule: "0 2 * * *"
      type: prompt
      command: "Run database backup"
      enabled: true`}</pre>
      </div>

      <div className="mt-4 flex items-center justify-center gap-4 text-[11px] text-text-muted">
        <span className="flex items-center gap-1.5">
          <Clock className="h-3 w-3 text-text-muted" />
          {t('jobs.cronExpression')}
        </span>
        <span className="h-3 w-px bg-border" />
        <span className="flex items-center gap-1.5">
          <FileCode className="h-3 w-3 text-text-muted" />
          {t('jobs.typePrompt')}, {t('jobs.typeCommand')}, {t('jobs.typeSkill')}
        </span>
      </div>
    </Card>
  )
}

function JobCard({ job }: { job: JobInfo }) {
  const { t } = useTranslation()
  const hasRun = job.last_run_at && job.last_run_at !== '0001-01-01T00:00:00Z'

  return (
    <Card
      padding="md"
      className={cn(
        'rounded-xl px-5 py-4 transition-colors',
        !job.enabled && 'opacity-60'
      )}
    >
      <div className="flex items-start gap-4">
        <div className={cn(
          'mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg',
          job.enabled ? 'bg-success-subtle' : 'bg-bg-subtle'
        )}>
          {job.enabled ? <Play className="h-4 w-4 text-success" /> : <Pause className="h-4 w-4 text-text-muted" />}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h3 className="text-sm font-semibold text-text-primary">{job.id}</h3>
            <span className="rounded bg-bg-subtle px-1.5 py-0.5 font-mono text-[10px] text-text-secondary">
              {job.schedule}
            </span>
            <Badge>{job.type}</Badge>
          </div>

          {job.command && (
            <pre className="mt-2 overflow-x-auto rounded-lg bg-bg-main px-3 py-2 font-mono text-xs text-text-secondary">
              {job.command}
            </pre>
          )}

          <div className="mt-2.5 flex flex-wrap items-center gap-3 text-[11px] text-text-muted">
            <span className="flex items-center gap-1">
              <Wrench className="h-3 w-3" />
              {t('jobs.runCount', { count: job.run_count })}
            </span>
            {hasRun && (
              <span className="flex items-center gap-1">
                <Timer className="h-3 w-3" />
                {t('jobs.lastRun', { time: timeAgo(job.last_run_at) })}
              </span>
            )}
            {job.last_error && (
              <span className="flex items-center gap-1 text-warning">
                <AlertCircle className="h-3 w-3" />
                {job.last_error}
              </span>
            )}
          </div>
        </div>
      </div>
    </Card>
  )
}
