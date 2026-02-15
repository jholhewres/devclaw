import { Shield, Key, Activity, Lock } from 'lucide-react'

/**
 * Painel de segurança — audit log, tool guard, API keys, sessões.
 * Fase inicial: placeholder visual que será incrementado.
 */
export function Security() {
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-8">
        <h1 className="text-xl font-semibold">Segurança</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Controle de acesso, auditoria e chaves de API
        </p>

        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          {/* Audit Log */}
          <SecurityCard
            icon={Activity}
            title="Audit Log"
            description="Histórico de ações executadas pelo assistente"
            status="Em breve"
          />

          {/* Tool Guard */}
          <SecurityCard
            icon={Shield}
            title="Tool Guard"
            description="Configure quais ferramentas precisam de aprovação"
            status="Em breve"
          />

          {/* API Keys */}
          <SecurityCard
            icon={Key}
            title="API Keys"
            description="Gerencie chaves de acesso ao gateway"
            status="Em breve"
          />

          {/* Vault */}
          <SecurityCard
            icon={Lock}
            title="Vault"
            description="Secrets criptografados armazenados com segurança"
            status="Em breve"
          />
        </div>
      </div>
    </div>
  )
}

function SecurityCard({
  icon: Icon,
  title,
  description,
  status,
}: {
  icon: React.ElementType
  title: string
  description: string
  status: string
}) {
  return (
    <div className="rounded-xl border border-zinc-200 p-5 dark:border-zinc-800">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-zinc-100 dark:bg-zinc-800">
          <Icon className="h-5 w-5 text-zinc-500" />
        </div>
        <div>
          <h3 className="text-sm font-medium">{title}</h3>
          <span className="text-[11px] text-zinc-400">{status}</span>
        </div>
      </div>
      <p className="mt-3 text-xs text-zinc-500">{description}</p>
    </div>
  )
}
