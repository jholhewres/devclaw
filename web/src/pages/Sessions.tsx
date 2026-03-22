import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Trash2, MessageSquare } from 'lucide-react';
import { api, type SessionInfo } from '@/lib/api';
import { Button } from '@/components/ui/Button';
import { timeAgo } from '@/lib/utils';
import { useAppStore } from '@/stores/app';
import { PageContainer } from '@/components/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { SearchInput } from '@/components/ui/SearchInput';
import { Badge } from '@/components/ui/Badge';
import { EmptyState } from '@/components/ui/EmptyState';
import { LoadingSpinner } from '@/components/ui/ConfigComponents';

const CHANNEL_LABELS: Record<string, string> = {
  whatsapp: 'WhatsApp',
  discord: 'Discord',
  telegram: 'Telegram',
  slack: 'Slack',
};

function getSessionLabel(session: SessionInfo): string {
  if (session.title) return session.title;
  const chatId = session.chat_id || session.id;
  const channel = session.channel || chatId.split(':')[0];
  if (channel === 'webui') return chatId.replace(/^webui:/, '');
  const prefix = CHANNEL_LABELS[channel] || channel;
  const chatPart = chatId.includes(':') ? chatId.split(':').slice(1).join(':') : chatId;
  return `${prefix}: ${chatPart}`;
}

export function Sessions() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [confirmingDelete, setConfirmingDelete] = useState<string | null>(null);

  useEffect(() => {
    api.sessions
      .list()
      .then((all) => {
        const sorted = (all || []).sort(
          (a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime()
        );
        setSessions(sorted);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const filtered = sessions.filter((s) => {
    const q = search.toLowerCase();
    return (
      getSessionLabel(s).toLowerCase().includes(q) ||
      s.channel.toLowerCase().includes(q) ||
      (s.chat_id && s.chat_id.toLowerCase().includes(q))
    );
  });

  const handleDelete = async (id: string) => {
    try {
      await api.sessions.delete(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
      useAppStore.getState().invalidateSessions();
    } catch {
      /* silent */
    } finally {
      setConfirmingDelete(null);
    }
  };

  if (loading) {
    return <LoadingSpinner />;
  }

  return (
    <PageContainer>
      <PageHeader
        title={t('sessions.title')}
        description={`${sessions.length} ${t('sessions.subtitle')}`}
      />

      {/* Search */}
      <SearchInput
        value={search}
        onChange={setSearch}
        placeholder={t('sessions.searchPlaceholder')}
        className="mt-5"
      />

      {/* List */}
      <div className="mt-4 flex flex-col gap-1">
        {filtered.map((session) => {
          const chatId = session.chat_id || session.id;
          const channel = session.channel || chatId.split(':')[0];
          const isExternal = channel !== 'webui';
          const label = getSessionLabel(session);

          return (
            <div
              key={session.id}
              className="group flex items-center rounded-xl transition-all duration-200 hover:bg-primary_hover"
            >
              <button
                onClick={() => navigate(`/chat/${encodeURIComponent(chatId)}`)}
                className="flex min-w-0 flex-1 cursor-pointer items-center gap-3 px-3 py-3 text-left"
              >
                <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-secondary text-quaternary">
                  <MessageSquare className="size-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    {isExternal && (
                      <Badge>
                        {CHANNEL_LABELS[channel] || channel}
                      </Badge>
                    )}
                    <p className="truncate text-sm font-medium text-secondary">{label}</p>
                  </div>
                  <p className="mt-0.5 text-xs text-tertiary">
                    {session.message_count} {t('sessions.messages')} ·{' '}
                    {timeAgo(session.last_message_at, t)}
                  </p>
                </div>
              </button>

              {confirmingDelete === session.id ? (
                <Button
                  variant="destructive-subtle"
                  size="xs"
                  className="mr-3 shrink-0"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleDelete(session.id);
                  }}
                >
                  {t('common.confirm')}
                </Button>
              ) : (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setConfirmingDelete(session.id);
                    setTimeout(() => setConfirmingDelete(null), 3000);
                  }}
                  aria-label="Delete session"
                  className="mr-3 shrink-0 cursor-pointer rounded-xl p-1.5 text-quaternary opacity-0 transition-all group-hover:opacity-100 hover:bg-error-primary hover:text-fg-error-secondary"
                >
                  <Trash2 className="size-4" />
                </button>
              )}
            </div>
          );
        })}

        {filtered.length === 0 && (
          <EmptyState
            icon={<MessageSquare className="size-6" />}
            title={search ? t('sessions.noResults') : t('sessions.noSessions')}
          />
        )}
      </div>
    </PageContainer>
  );
}
