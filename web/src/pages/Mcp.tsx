import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus, Trash2, Play, Square, RefreshCw, Server, Cable, AlertTriangle } from 'lucide-react';
import { api } from '@/lib/api';
import { Button } from '@/components/ui/Button';
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigToggle,
  ConfigCard,
  ConfigEmptyState,
  ConfigInfoBox,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents';

interface MCPServer {
  name: string;
  command: string;
  args: string[];
  env: Record<string, string>;
  enabled: boolean;
  status?: string;
  error?: string;
}

export function Mcp() {
  const { t } = useTranslation();
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
  const [newServer, setNewServer] = useState<Partial<MCPServer>>({
    name: '',
    command: '',
    args: [],
    env: {},
    enabled: true,
  });

  useEffect(() => {
    loadServers();
  }, []);

  const loadServers = async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const data = await api.mcp.list();
      setServers(data.servers || []);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load MCP servers');
    } finally {
      setLoading(false);
    }
  };

  const handleToggleServer = async (name: string, enabled: boolean) => {
    try {
      await api.mcp.update(name, enabled);
      setServers((prev) => prev.map((s) => (s.name === name ? { ...s, enabled } : s)));
      setMessage({
        type: 'success',
        text: enabled ? t('mcp.serverEnabled') : t('mcp.serverDisabled'),
      });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') });
    }
  };

  const handleStartServer = async (name: string) => {
    try {
      await api.mcp.start(name);
      setServers((prev) =>
        prev.map((s) => (s.name === name ? { ...s, status: 'running', error: undefined } : s))
      );
      setMessage({ type: 'success', text: t('mcp.serverStarted') });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') });
    }
  };

  const handleStopServer = async (name: string) => {
    try {
      await api.mcp.stop(name);
      setServers((prev) => prev.map((s) => (s.name === name ? { ...s, status: 'stopped' } : s)));
      setMessage({ type: 'success', text: t('mcp.serverStopped') });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') });
    }
  };

  const handleDeleteServer = async (name: string) => {
    try {
      await api.mcp.delete(name);
      setServers((prev) => prev.filter((s) => s.name !== name));
      setMessage({ type: 'success', text: t('mcp.serverDeleted') });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') });
    }
  };

  const handleAddServer = async () => {
    if (!newServer.name || !newServer.command) {
      setMessage({ type: 'error', text: t('mcp.nameRequired') });
      return;
    }

    try {
      await api.mcp.create(
        newServer.name,
        newServer.command,
        newServer.args || [],
        newServer.env || {}
      );
      const server: MCPServer = {
        name: newServer.name,
        command: newServer.command,
        args: newServer.args || [],
        env: newServer.env || {},
        enabled: newServer.enabled ?? true,
        status: 'stopped',
      };
      setServers((prev) => [...prev, server]);
      setNewServer({ name: '', command: '', args: [], env: {}, enabled: true });
      setShowAddForm(false);
      setMessage({ type: 'success', text: t('mcp.serverAdded') });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : t('common.error') });
    }
  };

  if (loading) return <LoadingSpinner />;
  if (loadError) return <ErrorState message={loadError} onRetry={loadServers} />;

  return (
    <ConfigPage
      title={t('mcp.title')}
      subtitle={t('mcp.subtitle')}
      description={t('mcp.description')}
      message={message}
      actions={
        <div className="flex items-center gap-3">
          <Button size="lg" variant="outline" onClick={() => loadServers()}>
            <RefreshCw className="h-4 w-4" />
            {t('mcp.refresh')}
          </Button>
          <Button size="lg" onClick={() => setShowAddForm(!showAddForm)}>
            <Plus className="h-4 w-4" />
            {t('mcp.addServer')}
          </Button>
        </div>
      }
    >
      {/* Add Server Form */}
      {showAddForm && (
        <ConfigSection title={t('mcp.newServer')}>
          <ConfigField label={t('mcp.serverName')}>
            <ConfigInput
              value={newServer.name || ''}
              onChange={(v) => setNewServer((prev) => ({ ...prev, name: v }))}
              placeholder="my-server"
            />
          </ConfigField>

          <ConfigField label={t('mcp.command')} hint={t('mcp.commandHint')}>
            <ConfigInput
              value={newServer.command || ''}
              onChange={(v) => setNewServer((prev) => ({ ...prev, command: v }))}
              placeholder="mcp-server-command"
            />
          </ConfigField>

          <div className="flex gap-3 pt-2">
            <Button className="bg-success hover:opacity-90" onClick={handleAddServer}>
              <Plus className="h-4 w-4" />
              {t('mcp.createServer')}
            </Button>
            <Button variant="outline" onClick={() => setShowAddForm(false)}>
              {t('common.cancel')}
            </Button>
          </div>
        </ConfigSection>
      )}

      {/* Servers List */}
      <ConfigSection
        icon={Cable}
        title={t('mcp.serversSection')}
        description={t('mcp.serversSectionDesc')}
      >
        {servers.length === 0 ? (
          <ConfigEmptyState
            icon={Server}
            title={t('mcp.noServers')}
            description={t('mcp.noServersHint')}
          />
        ) : (
          <div className="space-y-4">
            {servers.map((server) => (
              <ConfigCard
                key={server.name}
                title={server.name}
                subtitle={server.command}
                icon={Server}
                status={(() => {
                  if (server.status === 'running') return 'success';
                  if (server.status === 'error') return 'error';
                  return 'neutral';
                })()}
                actions={
                  <div className="flex items-center gap-2">
                    {server.status === 'running' ? (
                      <Button
                        size="xs"
                        variant="destructive-subtle"
                        onClick={() => handleStopServer(server.name)}
                      >
                        <Square className="h-3.5 w-3.5" />
                        {t('mcp.stop')}
                      </Button>
                    ) : (
                      <Button
                        size="xs"
                        className="bg-success-subtle text-success hover:bg-success/20"
                        onClick={() => handleStartServer(server.name)}
                      >
                        <Play className="h-3.5 w-3.5" />
                        {t('mcp.start')}
                      </Button>
                    )}
                    <Button
                      size="xs"
                      variant="ghost"
                      className="hover:text-error"
                      onClick={() => handleDeleteServer(server.name)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t('common.delete')}
                    </Button>
                  </div>
                }
              >
                {server.error && (
                  <div className="-mt-2 mb-4 flex items-start gap-2 rounded-xl border border-error/10 bg-error-subtle p-3">
                    <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-error" />
                    <p className="text-xs text-error">{server.error}</p>
                  </div>
                )}
                <ConfigToggle
                  enabled={server.enabled}
                  onChange={(v) => handleToggleServer(server.name, v)}
                  label={t('mcp.enabled')}
                />
              </ConfigCard>
            ))}
          </div>
        )}
      </ConfigSection>

      {/* Info */}
      <ConfigInfoBox
        title={t('mcp.info')}
        items={[t('mcp.info1'), t('mcp.info2'), t('mcp.info3')]}
      />
    </ConfigPage>
  );
}
