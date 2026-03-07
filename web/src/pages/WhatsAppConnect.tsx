import { useEffect, useRef, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
import {
  CheckCircle2,
  RefreshCw,
  Loader2,
  WifiOff,
  QrCode,
  Shield,
} from 'lucide-react'
import { api, type WhatsAppStatus } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Card } from '@/components/ui/Card'
import { StatusDot } from '@/components/ui/StatusDot'

/** Possible WhatsApp connection states */
type ConnectionState =
  | 'loading'
  | 'connected'
  | 'waiting_qr'
  | 'scanning'
  | 'timeout'
  | 'error'

/**
 * WhatsApp connection page via QR Code.
 * Uses SSE to receive QR codes in real time from the backend.
 */
export function WhatsAppConnect() {
  const { t } = useTranslation()
  const [state, setState] = useState<ConnectionState>('loading')
  const [qrCode, setQrCode] = useState<string>('')
  const [message, setMessage] = useState<string>('')
  const [refreshing, setRefreshing] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  /** Connect to SSE stream for QR events */
  const connectSSE = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
    }

    const token = localStorage.getItem('devclaw_token')
    const url = token
      ? `/api/channels/whatsapp/qr?token=${encodeURIComponent(token)}`
      : '/api/channels/whatsapp/qr'

    const es = new EventSource(url)
    eventSourceRef.current = es

    es.addEventListener('status', (e) => {
      const data: WhatsAppStatus = JSON.parse(e.data)
      if (data.connected) {
        setState('connected')
        setMessage(t('whatsapp.connected'))
      } else if (data.needs_qr) {
        setState('waiting_qr')
        setMessage(t('whatsapp.waitingQR'))
      }
    })

    es.addEventListener('code', (e) => {
      const data = JSON.parse(e.data)
      setQrCode(data.code)
      setState('waiting_qr')
      setMessage(data.message || t('whatsapp.scanQR'))
    })

    es.addEventListener('success', (e) => {
      const data = JSON.parse(e.data)
      setState('connected')
      setMessage(data.message || t('whatsapp.connected'))
      setQrCode('')
    })

    es.addEventListener('timeout', (e) => {
      const data = JSON.parse(e.data)
      setState('timeout')
      setMessage(data.message || t('whatsapp.qrExpired'))
      setQrCode('')
    })

    es.addEventListener('error', (e) => {
      if (e instanceof MessageEvent && e.data) {
        const data = JSON.parse(e.data)
        setState('error')
        setMessage(data.message || t('whatsapp.connectionError'))
      }
    })

    es.addEventListener('close', () => {
      es.close()
    })

    es.onerror = () => {
      setState('error')
      setMessage(t('whatsapp.sseLost'))
    }
  }, [t])

  useEffect(() => {
    api.channels.whatsapp.status()
      .then((status) => {
        if (status.connected) {
          setState('connected')
          setMessage(t('whatsapp.connected'))
        } else {
          connectSSE()
        }
      })
      .catch(() => {
        setState('error')
        setMessage(t('whatsapp.statusError'))
      })

    return () => {
      eventSourceRef.current?.close()
    }
  }, [connectSSE, t])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await api.channels.whatsapp.requestQR()
      setState('waiting_qr')
      setMessage(t('whatsapp.generatingQR'))
      setQrCode('')
      connectSSE()
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('whatsapp.connectionError')
      setMessage(msg)
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div className="flex-1 overflow-y-auto bg-bg-main">
      <div className="mx-auto max-w-2xl px-6 py-8">
        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-bg-subtle border border-border">
            <svg viewBox="0 0 24 24" className="h-5 w-5 text-success" fill="currentColor">
              <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
            </svg>
          </div>
          <div>
            <h1 className="text-xl font-semibold text-text-primary">{t('whatsapp.title')}</h1>
            <p className="text-sm text-text-muted">{t('whatsapp.subtitle')}</p>
          </div>
        </div>

        <div className="mt-8">
          {/* -- Connected -- */}
          {state === 'connected' && (
            <div className="flex flex-col items-center rounded-2xl border border-success/20 bg-success-subtle px-8 py-10">
              <div className="flex h-20 w-20 items-center justify-center rounded-full bg-success-subtle border border-success/30">
                <CheckCircle2 className="h-10 w-10 text-success" />
              </div>
              <h2 className="mt-5 text-lg font-semibold text-text-primary">{t('whatsapp.connected')}</h2>
              <p className="mt-1 text-sm text-success">{message}</p>
              <div className="mt-4">
                <StatusDot status="online" label={t('common.online')} pulse />
              </div>
            </div>
          )}

          {/* -- Loading -- */}
          {state === 'loading' && (
            <div className="flex flex-col items-center gap-4 py-16">
              <div className="h-8 w-8 animate-spin rounded-full border-2 border-bg-subtle border-t-brand" />
              <p className="text-sm text-text-muted">{t('whatsapp.checkingConnection')}</p>
            </div>
          )}

          {/* -- QR Code -- */}
          {(state === 'waiting_qr' || state === 'scanning') && (
            <div className="grid gap-8 md:grid-cols-[1fr_auto]">
              {/* QR */}
              <div className="flex flex-col items-center">
                <Card className="relative rounded-2xl p-5">
                  {/* Decorative corners */}
                  <div className="absolute -left-px -top-px h-6 w-6 rounded-tl-2xl border-l-2 border-t-2 border-brand" />
                  <div className="absolute -right-px -top-px h-6 w-6 rounded-tr-2xl border-r-2 border-t-2 border-brand" />
                  <div className="absolute -bottom-px -left-px h-6 w-6 rounded-bl-2xl border-b-2 border-l-2 border-brand" />
                  <div className="absolute -bottom-px -right-px h-6 w-6 rounded-br-2xl border-b-2 border-r-2 border-brand" />

                  {qrCode ? (
                    <div className="rounded-xl bg-white p-3">
                      <QRCodeSVG
                        value={qrCode}
                        size={240}
                        level="L"
                        bgColor="#ffffff"
                        fgColor="#000000"
                      />
                    </div>
                  ) : (
                    <div className="flex h-[264px] w-[264px] items-center justify-center">
                      <div className="flex flex-col items-center gap-3">
                        <QrCode className="h-12 w-12 animate-pulse text-text-muted" />
                        <p className="text-xs text-text-muted">{t('whatsapp.generatingQR')}</p>
                      </div>
                    </div>
                  )}
                </Card>

                {/* Refresh */}
                <button
                  onClick={handleRefresh}
                  disabled={refreshing}
                  className="mt-4 flex cursor-pointer items-center gap-1.5 text-xs text-text-muted transition-colors hover:text-text-primary disabled:opacity-50"
                >
                  <RefreshCw className={cn('h-3 w-3', refreshing && 'animate-spin')} />
                  {t('whatsapp.generateNew')}
                </button>
              </div>

              {/* Instructions */}
              <div className="flex flex-col justify-center space-y-5 md:min-w-[220px]">
                <h3 className="text-sm font-semibold text-text-primary">{t('whatsapp.howToConnect')}</h3>

                <div className="space-y-4">
                  <StepItem number={1} text={t('whatsapp.step1')} />
                  <StepItem number={2} text={t('whatsapp.step2')} />
                  <StepItem number={3} text={t('whatsapp.step3')} />
                  <StepItem number={4} text={t('whatsapp.step4')} />
                </div>

                <div className="mt-2 flex items-start gap-2 rounded-xl bg-bg-subtle px-3 py-2.5">
                  <Shield className="mt-0.5 h-3.5 w-3.5 shrink-0 text-text-muted" />
                  <p className="text-[11px] text-text-secondary">
                    {t('whatsapp.e2eHint')}
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* -- Timeout -- */}
          {state === 'timeout' && (
            <div className="flex flex-col items-center rounded-2xl border border-warning/20 bg-warning-subtle px-8 py-10">
              <div className="flex h-20 w-20 items-center justify-center rounded-full bg-warning-subtle border border-warning/30">
                <WifiOff className="h-10 w-10 text-warning" />
              </div>
              <h2 className="mt-5 text-lg font-semibold text-text-primary">{t('whatsapp.qrExpired')}</h2>
              <p className="mt-1 text-sm text-warning">{message}</p>
              <button
                onClick={handleRefresh}
                disabled={refreshing}
                className="mt-6 flex cursor-pointer items-center gap-2 rounded-xl bg-warning px-5 py-2.5 text-sm font-medium text-white transition-all hover:opacity-90 disabled:opacity-50"
              >
                {refreshing ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                {t('whatsapp.generateNewQR')}
              </button>
            </div>
          )}

          {/* -- Error -- */}
          {state === 'error' && (
            <div className="flex flex-col items-center rounded-2xl border border-error/20 bg-error-subtle px-8 py-10">
              <div className="flex h-20 w-20 items-center justify-center rounded-full bg-error-subtle border border-error/30">
                <WifiOff className="h-10 w-10 text-error" />
              </div>
              <h2 className="mt-5 text-lg font-semibold text-text-primary">{t('whatsapp.connectionError')}</h2>
              <p className="mt-1 text-sm text-error">{message}</p>
              <button
                onClick={handleRefresh}
                disabled={refreshing}
                className="mt-6 flex cursor-pointer items-center gap-2 rounded-xl bg-error px-5 py-2.5 text-sm font-medium text-white transition-all hover:opacity-90 disabled:opacity-50"
              >
                {refreshing ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <RefreshCw className="h-4 w-4" />
                )}
                {t('whatsapp.tryAgain')}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/** Numbered step item for instructions */
function StepItem({ number, text }: { number: number; text: string }) {
  return (
    <div className="flex items-start gap-3">
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-subtle text-[11px] font-semibold text-brand">
        {number}
      </div>
      <p className="text-sm text-text-secondary leading-relaxed">
        {text}
      </p>
    </div>
  )
}
