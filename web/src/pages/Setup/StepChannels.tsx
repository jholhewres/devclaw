import { MessageSquare } from 'lucide-react'
import type { SetupData } from './SetupWizard'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const CHANNELS = [
  {
    id: 'whatsapp',
    name: 'WhatsApp',
    description: 'Conecte via QR code usando whatsmeow',
    icon: '💬',
  },
  {
    id: 'telegram',
    name: 'Telegram',
    description: 'Conecte usando token do BotFather',
    icon: '✈️',
  },
  {
    id: 'discord',
    name: 'Discord',
    description: 'Conecte usando bot token',
    icon: '🎮',
  },
  {
    id: 'slack',
    name: 'Slack',
    description: 'Conecte usando Slack App token',
    icon: '💼',
  },
]

/**
 * Etapa 4: Toggle de canais que serão ativados.
 */
export function StepChannels({ data, updateData }: Props) {
  const toggleChannel = (id: string) => {
    updateData({
      channels: {
        ...data.channels,
        [id]: !data.channels[id],
      },
    })
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-medium">Canais</h2>
        <p className="mt-1 text-sm text-zinc-500">
          Escolha onde o assistente vai responder
        </p>
      </div>

      <div className="space-y-2">
        {CHANNELS.map((ch) => (
          <label
            key={ch.id}
            className={`flex cursor-pointer items-center gap-3 rounded-lg border px-4 py-3 transition-colors ${
              data.channels[ch.id]
                ? 'border-zinc-900 bg-zinc-50 dark:border-zinc-400 dark:bg-zinc-800'
                : 'border-zinc-200 hover:border-zinc-300 dark:border-zinc-700 dark:hover:border-zinc-600'
            }`}
          >
            <input
              type="checkbox"
              checked={!!data.channels[ch.id]}
              onChange={() => toggleChannel(ch.id)}
              className="h-4 w-4 rounded border-zinc-300"
            />
            <span className="text-xl">{ch.icon}</span>
            <div>
              <span className="text-sm font-medium">{ch.name}</span>
              <p className="text-xs text-zinc-500">{ch.description}</p>
            </div>
          </label>
        ))}
      </div>

      <div className="flex items-center gap-2 rounded-lg bg-zinc-50 px-3 py-2 dark:bg-zinc-800/50">
        <MessageSquare className="h-4 w-4 text-zinc-400" />
        <p className="text-xs text-zinc-500">
          O chat web está sempre disponível, independente dos canais selecionados.
        </p>
      </div>
    </div>
  )
}
