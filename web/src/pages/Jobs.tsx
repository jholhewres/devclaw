import { useEffect, useState } from 'react'
import { Clock, Play, Pause, AlertCircle } from 'lucide-react'
import { api, type JobInfo } from '@/lib/api'
import { Badge } from '@/components/ui/Badge'
import { timeAgo } from '@/lib/utils'

/**
 * Cron Jobs — lista de jobs agendados com status e histórico.
 */
export function Jobs() {
  const [jobs, setJobs] = useState<JobInfo[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.jobs.list()
      .then(setJobs)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-xl font-semibold">Jobs</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Tarefas agendadas do assistente
        </p>

        <div className="mt-6 space-y-3">
          {jobs.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-zinc-300 py-12 dark:border-zinc-700">
              <Clock className="h-8 w-8 text-zinc-300 dark:text-zinc-600" />
              <p className="mt-3 text-sm text-zinc-500">Nenhum job agendado</p>
              <p className="mt-1 text-xs text-zinc-400">
                Configure jobs no arquivo config.yaml
              </p>
            </div>
          ) : (
            jobs.map((job) => (
              <div
                key={job.id}
                className="rounded-xl border border-zinc-200 px-5 py-4 dark:border-zinc-800"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`flex h-8 w-8 items-center justify-center rounded-lg ${
                      job.enabled
                        ? 'bg-emerald-50 dark:bg-emerald-900/20'
                        : 'bg-zinc-100 dark:bg-zinc-800'
                    }`}>
                      {job.enabled ? (
                        <Play className="h-4 w-4 text-emerald-500" />
                      ) : (
                        <Pause className="h-4 w-4 text-zinc-400" />
                      )}
                    </div>
                    <div>
                      <h3 className="text-sm font-medium">{job.id}</h3>
                      <p className="text-xs text-zinc-500">
                        <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">
                          {job.schedule}
                        </code>
                        {' · '}
                        {job.type}
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <Badge variant={job.enabled ? 'success' : 'default'}>
                      {job.enabled ? 'Ativo' : 'Pausado'}
                    </Badge>
                  </div>
                </div>

                <div className="mt-3 flex items-center gap-4 text-xs text-zinc-500">
                  <span>{job.run_count} execuções</span>
                  {job.last_run_at && job.last_run_at !== '0001-01-01T00:00:00Z' && (
                    <span>Última: {timeAgo(job.last_run_at)}</span>
                  )}
                  {job.last_error && (
                    <span className="flex items-center gap-1 text-amber-500">
                      <AlertCircle className="h-3 w-3" />
                      {job.last_error}
                    </span>
                  )}
                </div>

                {job.command && (
                  <pre className="mt-2 rounded-lg bg-zinc-50 px-3 py-2 text-xs text-zinc-600 dark:bg-zinc-800/50 dark:text-zinc-400">
                    {job.command}
                  </pre>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
