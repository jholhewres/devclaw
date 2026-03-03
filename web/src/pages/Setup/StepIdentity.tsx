import { useTranslation } from 'react-i18next'
import { User, Globe, Clock } from 'lucide-react'
import type { SetupData } from './SetupWizard'
import { StepContainer, StepHeader, FieldGroup, Field, Input, Select } from './SetupComponents'

interface Props {
  data: SetupData
  updateData: (partial: Partial<SetupData>) => void
}

const LANGUAGES = [
  { value: 'pt-BR', label: 'Português (Brasil)', flag: '🇧🇷' },
  { value: 'en', label: 'English', flag: '🇺🇸' },
  { value: 'es', label: 'Español', flag: '🇪🇸' },
]

// Common timezones organized by region
const TIMEZONES = [
  { value: 'UTC', label: 'UTC' },
  { value: 'America/Sao_Paulo', label: 'Brasília (GMT-3)' },
  { value: 'America/Manaus', label: 'Manaus (GMT-4)' },
  { value: 'America/Belem', label: 'Belém (GMT-3)' },
  { value: 'America/Fortaleza', label: 'Fortaleza (GMT-3)' },
  { value: 'America/New_York', label: 'New York (EST)' },
  { value: 'America/Los_Angeles', label: 'Los Angeles (PST)' },
  { value: 'America/Chicago', label: 'Chicago (CST)' },
  { value: 'America/Denver', label: 'Denver (MST)' },
  { value: 'America/Toronto', label: 'Toronto (EST)' },
  { value: 'America/Mexico_City', label: 'Cidade do México' },
  { value: 'Europe/London', label: 'Londres (GMT)' },
  { value: 'Europe/Paris', label: 'Paris (CET)' },
  { value: 'Europe/Berlin', label: 'Berlim (CET)' },
  { value: 'Europe/Madrid', label: 'Madrid (CET)' },
  { value: 'Europe/Lisbon', label: 'Lisboa (WET)' },
  { value: 'Asia/Tokyo', label: 'Tóquio (JST)' },
  { value: 'Asia/Shanghai', label: 'Shanghai (CST)' },
  { value: 'Asia/Dubai', label: 'Dubai (GST)' },
  { value: 'Asia/Singapore', label: 'Singapura (SGT)' },
  { value: 'Australia/Sydney', label: 'Sydney (AEST)' },
]

export function StepIdentity({ data, updateData }: Props) {
  const { t, i18n } = useTranslation()

  const handleLanguageChange = (lang: string) => {
    updateData({ language: lang })
    i18n.changeLanguage(lang)
  }

  return (
    <StepContainer>
      <StepHeader
        title={t('setupPage.identityTitle')}
        description={t('setupPage.identityDesc')}
      />

      <FieldGroup>
        <Field label={t('setupPage.assistantName')} icon={User}>
          <Input
            value={data.name}
            onChange={(val) => updateData({ name: val })}
            placeholder="DevClaw"
          />
        </Field>

        <Field label={t('setupPage.language')} icon={Globe}>
          <div className="grid grid-cols-3 gap-2">
            {LANGUAGES.map((lang) => (
              <button
                key={lang.value}
                onClick={() => handleLanguageChange(lang.value)}
                className={`flex cursor-pointer items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition-all ${
                  data.language === lang.value
                    ? 'border-[#6366f1]/50 bg-[#6366f1]/10 text-[#f1f5f9]'
                    : 'border-[rgba(99,102,241,0.12)] bg-[#0b0d17] text-[#94a3b8] hover:border-[rgba(99,102,241,0.24)] hover:bg-[#14172b]'
                }`}
              >
                <span className="text-base">{lang.flag}</span>
                <span className="text-xs font-medium">{lang.label}</span>
              </button>
            ))}
          </div>
        </Field>

        <Field label={t('setupPage.timezone')} icon={Clock} hint={t('setupPage.timezoneHint')}>
          <Select
            value={data.timezone}
            onChange={(val) => updateData({ timezone: val })}
            options={TIMEZONES}
          />
        </Field>
      </FieldGroup>
    </StepContainer>
  )
}
