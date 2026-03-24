import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { MessageSquare, Plus } from 'lucide-react';
import { Button as AriaButton } from 'react-aria-components';
import { Tooltip } from '@/components/base/tooltip/tooltip';
import { cx } from '@/utils/cx';
import { useAppStore } from '@/stores/app';
import { api, type SessionInfo } from '@/lib/api';
import { timeAgo } from '@/lib/utils';

const MAX_SESSIONS = 5;

const channelLabels: Record<string, string> = {
  whatsapp: 'WhatsApp',
  discord: 'Discord',
  telegram: 'Telegram',
  slack: 'Slack',
};

interface ConversationListProps {
  collapsed?: boolean;
}

export function ConversationList({ collapsed }: ConversationListProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const sessionVersion = useAppStore((s) => s.sessionVersion);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);

  const [sessions, setSessions] = useState<SessionInfo[]>([]);

  useEffect(() => {
    api.sessions
      .list()
      .then((all) => {
        const sorted = (all || [])
          .filter((s) => s.channel === 'webui' || s.id.startsWith('webui:'))
          .sort(
            (a, b) =>
              new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime()
          )
          .slice(0, MAX_SESSIONS);
        setSessions(sorted);
      })
      .catch(() => {});
  }, [location.pathname, sessionVersion]);

  const handleNewConversation = () => {
    useAppStore.getState().setActiveSession(null);
    navigate('/');
    setSidebarOpen(false);
  };

  const handleNavigate = (sessionId: string) => {
    navigate(`/chat/${encodeURIComponent(sessionId)}`);
    setSidebarOpen(false);
  };

  // Collapsed mode: show icon or just a spacer
  if (collapsed) {
    if (sessions.length === 0) return <div className="flex-1" />;
    return (
      <div className="flex-1 px-3 pt-4">
        <Tooltip title={t('sidebar.recent')} placement="right" delay={200}>
          <AriaButton
            onPress={() => {
              navigate('/sessions');
              setSidebarOpen(false);
            }}
            className="flex w-full items-center justify-center rounded-lg px-3 py-2 text-fg-quaternary outline-hidden transition-colors hover:bg-primary_hover hover:text-fg-tertiary cursor-pointer"
          >
            <MessageSquare className="size-5" />
          </AriaButton>
        </Tooltip>
      </div>
    );
  }

  // Expanded: always render flex-1 container to push footer to bottom
  if (sessions.length === 0) return <div className="flex-1" />;

  return (
    <div className="flex-1 overflow-y-auto px-3 pt-4">
      <div className="border-t border-secondary pt-4">
        {/* Header */}
        <div className="mb-2 flex items-center justify-between px-3">
          <span className="text-xs font-semibold uppercase tracking-wider text-quaternary">
            {t('sidebar.recent')}
          </span>
          <AriaButton
            onPress={handleNewConversation}
            className="rounded-md p-1 text-fg-quaternary outline-hidden transition-colors hover:bg-primary_hover hover:text-fg-tertiary cursor-pointer"
            aria-label={t('sidebar.newConversation')}
          >
            <Plus className="size-3.5" />
          </AriaButton>
        </div>

        {/* Session list */}
        <div className="space-y-0.5">
          {sessions.map((session) => {
            const navId = session.chat_id || session.id;
            const active = location.pathname === `/chat/${encodeURIComponent(navId)}`;
            const channel = session.channel && channelLabels[session.channel];

            return (
              <AriaButton
                key={session.id}
                onPress={() => handleNavigate(navId)}
                className={cx(
                  'flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left outline-hidden transition-all duration-150 cursor-pointer',
                  active
                    ? 'bg-active text-secondary_hover'
                    : 'text-tertiary hover:bg-primary_hover hover:text-secondary_hover',
                )}
              >
                <MessageSquare className="size-3.5 shrink-0 text-fg-quaternary" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium">
                    {session.title || session.id.replace('webui:', '')}
                  </p>
                  <p className="text-[10px] text-quaternary">
                    {channel && <span className="mr-1">{channel}</span>}
                    {timeAgo(session.last_message_at, t)}
                  </p>
                </div>
              </AriaButton>
            );
          })}
        </div>

        {/* See all link */}
        <AriaButton
          onPress={() => {
            navigate('/sessions');
            setSidebarOpen(false);
          }}
          className="mt-2 flex w-full items-center justify-center rounded-lg px-3 py-1.5 text-xs font-medium text-fg-quaternary outline-hidden transition-colors hover:bg-primary_hover hover:text-fg-tertiary cursor-pointer"
        >
          {t('common.viewAll')}
        </AriaButton>
      </div>
    </div>
  );
}
