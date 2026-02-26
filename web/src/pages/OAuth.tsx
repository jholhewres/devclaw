import { useTranslation } from 'react-i18next'
import { Link2 } from 'lucide-react'
import { OAuthSettings } from '@/components/OAuthSettings'

export function OAuth() {
  const { t } = useTranslation()

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-[#0c1222]">
      <div className="mx-auto w-full max-w-4xl flex-1 overflow-y-auto px-4 py-12 sm:px-6 sm:py-16 lg:px-8">
        {/* Header */}
        <div className="flex items-start justify-between mb-8">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-white/10 bg-[#111827]">
              <Link2 className="h-5 w-5 text-[#64748b]" />
            </div>
            <div>
              <p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[#475569]">{t('sidebar.settings')}</p>
              <h1 className="mt-1 text-2xl font-bold text-[#f8fafc] tracking-tight">{t('sidebar.oauth')}</h1>
            </div>
          </div>
        </div>

        {/* OAuth Settings Component */}
        <OAuthSettings />
      </div>
    </div>
  )
}

export default OAuth
