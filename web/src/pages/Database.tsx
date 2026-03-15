import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import {
  Database,
  Server,
  Activity,
  HardDrive,
  AlertTriangle,
  CheckCircle2,
  RefreshCw,
} from 'lucide-react';
import { api } from '@/lib/api';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import {
  ConfigPage,
  ConfigSection,
  ConfigField,
  ConfigInput,
  ConfigSelect,
  ConfigToggle,
  ConfigActions,
  LoadingSpinner,
  ErrorState,
} from '@/components/ui/ConfigComponents';

interface DatabaseStatus {
  name: string;
  healthy: boolean;
  latency: number;
  version: string;
  error?: string;
  open_connections: number;
  in_use: number;
  idle: number;
  wait_count: number;
  wait_duration: number;
  max_open_conns: number;
}

interface DatabaseConfig {
  backend: string;
  sqlite: {
    path: string;
    journal_mode: string;
    busy_timeout: number;
    foreign_keys: boolean;
  };
  postgresql: {
    host: string;
    port: number;
    database: string;
    user: string;
    ssl_mode: string;
    max_open_conns: number;
    max_idle_conns: number;
    conn_max_lifetime: string;
    vector_enabled: boolean;
    vector_dimensions: number;
    vector_index_type: string;
  };
}

const BACKEND_KEYS = [
  { value: 'sqlite', key: 'database.backendSqlite' },
  { value: 'postgresql', key: 'database.backendPostgresql' },
];

const JOURNAL_MODE_KEYS = [
  { value: 'WAL', key: 'database.journalWal' },
  { value: 'DELETE', key: 'database.journalDelete' },
  { value: 'TRUNCATE', key: 'database.journalTruncate' },
  { value: 'PERSIST', key: 'database.journalPersist' },
  { value: 'MEMORY', key: 'database.journalMemory' },
];

const SSL_MODE_KEYS = [
  { value: 'disable', key: 'database.sslDisable' },
  { value: 'require', key: 'database.sslRequire' },
  { value: 'verify-ca', key: 'database.sslVerifyCa' },
  { value: 'verify-full', key: 'database.sslVerifyFull' },
];

const INDEX_TYPE_KEYS = [
  { value: 'hnsw', key: 'database.indexHnsw' },
  { value: 'ivfflat', key: 'database.indexIvfflat' },
];

// Status card component
function StatusCard({
  label,
  value,
  subtext,
  icon: Icon,
  status,
}: {
  label: string;
  value: string | number;
  subtext?: string;
  icon?: React.ElementType;
  status?: 'success' | 'error' | 'neutral';
}) {
  const statusColors = {
    success: 'text-success',
    error: 'text-error',
    neutral: 'text-text-primary',
  };

  return (
    <Card padding="md">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs tracking-wide text-text-muted uppercase">{label}</span>
        {Icon && (
          <Icon
            className={cn(
              'h-4 w-4',
              status === 'success' ? 'text-success' : status === 'error' ? 'text-error' : 'text-text-muted'
            )}
          />
        )}
      </div>
      <p
        className={cn(
          'text-lg font-semibold',
          status && statusColors[status],
          !status && 'text-text-primary'
        )}
      >
        {value}
      </p>
      {subtext && <p className="mt-1 text-xs text-text-muted">{subtext}</p>}
    </Card>
  );
}

export function DatabasePage() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<DatabaseStatus | null>(null);
  const [config, setConfig] = useState<DatabaseConfig | null>(null);
  const [original, setOriginal] = useState<DatabaseConfig | null>(null);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const [statusData, configData] = await Promise.all([api.database.status(), api.config.get()]);

      setStatus(statusData);

      const dbConfig = (configData as unknown as { database?: DatabaseConfig }).database || {
        backend: 'sqlite',
        sqlite: {
          path: './data/devclaw.db',
          journal_mode: 'WAL',
          busy_timeout: 5000,
          foreign_keys: true,
        },
        postgresql: {
          host: 'localhost',
          port: 5432,
          database: 'devclaw',
          user: 'devclaw',
          ssl_mode: 'disable',
          max_open_conns: 25,
          max_idle_conns: 10,
          conn_max_lifetime: '30m',
          vector_enabled: true,
          vector_dimensions: 1536,
          vector_index_type: 'hnsw',
        },
      };
      setConfig(dbConfig);
      setOriginal(JSON.parse(JSON.stringify(dbConfig)));
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : t('database.loadFailed'));
    } finally {
      setLoading(false);
    }
  };

  const hasChanges = JSON.stringify(config) !== JSON.stringify(original);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setMessage(null);
    try {
      await api.config.update({ database: config });
      setOriginal(JSON.parse(JSON.stringify(config)));
      setMessage({ type: 'success', text: t('common.success') });
    } catch {
      setMessage({ type: 'error', text: t('common.error') });
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    if (original) {
      setConfig(JSON.parse(JSON.stringify(original)));
    }
    setMessage(null);
  };

  if (loading) return <LoadingSpinner />;
  if (loadError || !config)
    return <ErrorState message={loadError || undefined} onRetry={loadData} />;

  return (
    <ConfigPage
      title={t('database.title')}
      subtitle={t('database.subtitle')}
      description={t('database.description')}
      message={message}
      actions={
        <div className="flex items-center gap-3">
          <Button size="lg" variant="outline" onClick={() => loadData()}>
            <RefreshCw className="h-4 w-4" />
            {t('database.refresh')}
          </Button>
          <ConfigActions
            onSave={handleSave}
            onReset={handleReset}
            saving={saving}
            hasChanges={hasChanges}
            saveLabel={t('common.save')}
            savingLabel={t('common.saving')}
            resetLabel={t('common.reset')}
          />
        </div>
      }
    >
      {/* Status Cards */}
      {status && (
        <ConfigSection
          icon={Activity}
          title={t('database.statusSection')}
          description={t('database.statusSectionDesc')}
        >
          <div className="-mt-2 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatusCard
              label={t('database.health')}
              value={status.healthy ? t('database.healthy') : t('database.unhealthy')}
              subtext={`${status.latency}ms`}
              icon={status.healthy ? CheckCircle2 : AlertTriangle}
              status={status.healthy ? 'success' : 'error'}
            />
            <StatusCard
              label={t('database.backend')}
              value={status.name}
              subtext={`v${status.version}`}
              icon={Database}
            />
            <StatusCard
              label={t('database.connections')}
              value={status.open_connections}
              subtext={t('database.connectionStats', { inUse: status.in_use, idle: status.idle })}
              icon={Server}
            />
            <StatusCard
              label={t('database.poolSize')}
              value={status.max_open_conns}
              subtext={t('database.waitStats', { count: status.wait_count })}
              icon={HardDrive}
            />
          </div>
          {status.error && (
            <div className="mt-4 flex items-start gap-2 rounded-xl border border-error/10 bg-error-subtle p-3">
              <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-error" />
              <p className="text-xs text-error">{status.error}</p>
            </div>
          )}
        </ConfigSection>
      )}

      {/* Backend Selection */}
      <ConfigSection
        icon={Database}
        title={t('database.backendSection')}
        description={t('database.backendSectionDesc')}
      >
        <ConfigField label={t('database.backendType')} hint={t('database.backendTypeHint')}>
          <ConfigSelect
            value={config.backend}
            onChange={(v) => setConfig((prev) => (prev ? { ...prev, backend: v } : prev))}
            options={BACKEND_KEYS.map(o => ({ value: o.value, label: t(o.key) }))}
          />
        </ConfigField>
      </ConfigSection>

      {/* SQLite Config */}
      {config.backend === 'sqlite' && (
        <ConfigSection
          icon={HardDrive}
          title={t('database.sqliteSection')}
          description={t('database.sqliteSectionDesc')}
        >
          <ConfigField label={t('database.sqlitePath')}>
            <ConfigInput
              value={config.sqlite.path}
              onChange={(v) =>
                setConfig((prev) =>
                  prev ? { ...prev, sqlite: { ...prev.sqlite, path: v } } : prev
                )
              }
              placeholder="./data/devclaw.db"
            />
          </ConfigField>

          <ConfigField label={t('database.journalMode')}>
            <ConfigSelect
              value={config.sqlite.journal_mode}
              onChange={(v) =>
                setConfig((prev) =>
                  prev ? { ...prev, sqlite: { ...prev.sqlite, journal_mode: v } } : prev
                )
              }
              options={JOURNAL_MODE_KEYS.map(o => ({ value: o.value, label: t(o.key) }))}
            />
          </ConfigField>

          <ConfigField label={t('database.busyTimeout')} hint={t('database.busyTimeoutHint')}>
            <ConfigInput
              type="number"
              value={config.sqlite.busy_timeout}
              onChange={(v) =>
                setConfig((prev) =>
                  prev
                    ? { ...prev, sqlite: { ...prev.sqlite, busy_timeout: parseInt(v) || 5000 } }
                    : prev
                )
              }
              placeholder="5000"
            />
          </ConfigField>

          <ConfigToggle
            enabled={config.sqlite.foreign_keys}
            onChange={(v) =>
              setConfig((prev) =>
                prev ? { ...prev, sqlite: { ...prev.sqlite, foreign_keys: v } } : prev
              )
            }
            label={t('database.foreignKeys')}
          />
        </ConfigSection>
      )}

      {/* PostgreSQL Config */}
      {config.backend === 'postgresql' && (
        <ConfigSection
          icon={Server}
          title={t('database.postgresqlSection')}
          description={t('database.postgresqlSectionDesc')}
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <ConfigField label={t('database.host')}>
              <ConfigInput
                value={config.postgresql.host}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev ? { ...prev, postgresql: { ...prev.postgresql, host: v } } : prev
                  )
                }
                placeholder="localhost"
              />
            </ConfigField>

            <ConfigField label={t('database.port')}>
              <ConfigInput
                type="number"
                value={config.postgresql.port}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev
                      ? { ...prev, postgresql: { ...prev.postgresql, port: parseInt(v) || 5432 } }
                      : prev
                  )
                }
                placeholder="5432"
              />
            </ConfigField>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <ConfigField label={t('database.database')}>
              <ConfigInput
                value={config.postgresql.database}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev ? { ...prev, postgresql: { ...prev.postgresql, database: v } } : prev
                  )
                }
                placeholder="devclaw"
              />
            </ConfigField>

            <ConfigField label={t('database.user')}>
              <ConfigInput
                value={config.postgresql.user}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev ? { ...prev, postgresql: { ...prev.postgresql, user: v } } : prev
                  )
                }
                placeholder="devclaw"
              />
            </ConfigField>
          </div>

          <ConfigField label={t('database.sslMode')}>
            <ConfigSelect
              value={config.postgresql.ssl_mode}
              onChange={(v) =>
                setConfig((prev) =>
                  prev ? { ...prev, postgresql: { ...prev.postgresql, ssl_mode: v } } : prev
                )
              }
              options={SSL_MODE_KEYS.map(o => ({ value: o.value, label: t(o.key) }))}
            />
          </ConfigField>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <ConfigField label={t('database.maxOpenConns')}>
              <ConfigInput
                type="number"
                value={config.postgresql.max_open_conns}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev
                      ? {
                          ...prev,
                          postgresql: { ...prev.postgresql, max_open_conns: parseInt(v) || 25 },
                        }
                      : prev
                  )
                }
                placeholder="25"
              />
            </ConfigField>

            <ConfigField label={t('database.maxIdleConns')}>
              <ConfigInput
                type="number"
                value={config.postgresql.max_idle_conns}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev
                      ? {
                          ...prev,
                          postgresql: { ...prev.postgresql, max_idle_conns: parseInt(v) || 10 },
                        }
                      : prev
                  )
                }
                placeholder="10"
              />
            </ConfigField>

            <ConfigField label={t('database.connMaxLifetime')}>
              <ConfigInput
                value={config.postgresql.conn_max_lifetime}
                onChange={(v) =>
                  setConfig((prev) =>
                    prev
                      ? { ...prev, postgresql: { ...prev.postgresql, conn_max_lifetime: v } }
                      : prev
                  )
                }
                placeholder="30m"
              />
            </ConfigField>
          </div>
        </ConfigSection>
      )}

      {/* Vector Search (PostgreSQL only) */}
      {config.backend === 'postgresql' && (
        <ConfigSection
          icon={Activity}
          title={t('database.vectorSection')}
          description={t('database.vectorSectionDesc')}
          className="mb-10"
        >
          <ConfigToggle
            enabled={config.postgresql.vector_enabled}
            onChange={(v) =>
              setConfig((prev) =>
                prev ? { ...prev, postgresql: { ...prev.postgresql, vector_enabled: v } } : prev
              )
            }
            label={t('database.vectorEnabled')}
          />

          {config.postgresql.vector_enabled && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <ConfigField
                label={t('database.vectorDimensions')}
                hint={t('database.vectorDimensionsHint')}
              >
                <ConfigInput
                  type="number"
                  value={config.postgresql.vector_dimensions}
                  onChange={(v) =>
                    setConfig((prev) =>
                      prev
                        ? {
                            ...prev,
                            postgresql: {
                              ...prev.postgresql,
                              vector_dimensions: parseInt(v) || 1536,
                            },
                          }
                        : prev
                    )
                  }
                  placeholder="1536"
                />
              </ConfigField>

              <ConfigField
                label={t('database.vectorIndexType')}
                hint={t('database.vectorIndexTypeHint')}
              >
                <ConfigSelect
                  value={config.postgresql.vector_index_type}
                  onChange={(v) =>
                    setConfig((prev) =>
                      prev
                        ? { ...prev, postgresql: { ...prev.postgresql, vector_index_type: v } }
                        : prev
                    )
                  }
                  options={INDEX_TYPE_KEYS.map(o => ({ value: o.value, label: t(o.key) }))}
                />
              </ConfigField>
            </div>
          )}
        </ConfigSection>
      )}
    </ConfigPage>
  );
}
