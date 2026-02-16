import { useEffect, useState } from 'react'
import { Clock, Play, Pause, AlertCircle, Timer, Wrench } from 'lucide-react'
import { api, type JobInfo } from '@/lib/api'
import { timeAgo } from '@/lib/utils'

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
      <div className="flex flex-1 items-center justify-center bg-[#0a0a0f]">
        <div className="h-10 w-10 rounded-full border-4 border-orange-500/30 border-t-orange-500 animate-spin" />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto bg-[#0a0a0f]">
      <div className="mx-auto max-w-5xl px-8 py-10">
        <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-gray-600">Automacao</p>
        <h1 className="mt-1 text-2xl font-black text-white tracking-tight">Jobs</h1>
        <p className="mt-2 text-base text-gray-500">Tarefas agendadas do assistente</p>

        <div className="mt-8 space-y-4">
          {jobs.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-white/[0.08] py-20">
              <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-white/[0.04]">
                <Clock className="h-8 w-8 text-gray-700" />
              </div>
              <p className="mt-4 text-lg font-semibold text-gray-500">Nenhum job agendado</p>
              <p className="mt-1 text-sm text-gray-600">Configure jobs no config.yaml</p>
            </div>
          ) : (
            jobs.map((job) => (
              <div
                key={job.id}
                className={`relative overflow-hidden rounded-2xl border p-6 transition-all ${
                  job.enabled
                    ? 'border-emerald-500/25 bg-emerald-500/[0.03]'
                    : 'border-white/[0.06] bg-[#111118]'
                }`}
              >
                {job.enabled && (
                  <div className="absolute right-5 top-5">
                    <span className="rounded-full bg-emerald-500 px-3 py-1 text-[10px] font-bold text-white">ativo</span>
                  </div>
                )}

                <div className="flex items-start gap-5">
                  <div className={`flex h-14 w-14 shrink-0 items-center justify-center rounded-xl ${
                    job.enabled ? 'bg-emerald-500/15 text-emerald-400' : 'bg-white/[0.05] text-gray-500'
                  }`}>
                    {job.enabled ? <Play className="h-7 w-7" /> : <Pause className="h-7 w-7" />}
                  </div>
                  <div className="flex-1">
                    <h3 className="text-xl font-bold text-white">{job.id}</h3>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <span className="rounded-full bg-white/[0.04] px-3 py-1 font-mono text-xs font-bold text-orange-400">
                        {job.schedule}
                      </span>
                      <span className="rounded-full bg-white/[0.04] px-3 py-1 text-xs font-semibold text-gray-500">
                        {job.type}
                      </span>
                    </div>

                    <div className="mt-4 flex items-center gap-5 text-sm text-gray-500">
                      <span className="flex items-center gap-1.5">
                        <Wrench className="h-3.5 w-3.5" />
                        {job.run_count} execucoes
                      </span>
                      {job.last_run_at && job.last_run_at !== '0001-01-01T00:00:00Z' && (
                        <span className="flex items-center gap-1.5">
                          <Timer className="h-3.5 w-3.5" />
                          Ultima: {timeAgo(job.last_run_at)}
                        </span>
                      )}
                      {job.last_error && (
                        <span className="flex items-center gap-1.5 text-amber-400">
                          <AlertCircle className="h-3.5 w-3.5" />
                          {job.last_error}
                        </span>
                      )}
                    </div>

                    {job.command && (
                      <pre className="mt-4 overflow-x-auto rounded-xl border border-white/[0.04] bg-[#0a0a0f] px-5 py-3 font-mono text-sm text-gray-400">
                        {job.command}
                      </pre>
                    )}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
