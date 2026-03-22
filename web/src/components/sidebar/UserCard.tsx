import { useTranslation } from 'react-i18next';
import {
  Button as AriaButton,
  MenuTrigger as AriaMenuTrigger,
} from 'react-aria-components';
import { Settings, LogOut, Sun, Moon, Monitor, ChevronUp } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { Dropdown } from '@/components/base/dropdown/dropdown';
import { Tooltip } from '@/components/base/tooltip/tooltip';
import { cx } from '@/utils/cx';
import { useAppStore, type Theme } from '@/stores/app';
import { languages } from '@/i18n';

interface UserCardProps {
  collapsed?: boolean;
}

export function UserCard({ collapsed }: UserCardProps) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const theme = useAppStore((s) => s.theme);
  const setTheme = useAppStore((s) => s.setTheme);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);

  const handleLogout = () => {
    localStorage.removeItem('devclaw_token');
    window.location.href = '/login';
  };

  const handleSettings = () => {
    navigate('/system');
    setSidebarOpen(false);
  };

  const changeLanguage = (code: string) => {
    i18n.changeLanguage(code);
    localStorage.setItem('devclaw_language', code);
  };

  const themeOptions: { value: Theme; icon: typeof Sun; labelKey: string }[] = [
    { value: 'light', icon: Sun, labelKey: 'userMenu.themeLight' },
    { value: 'dark', icon: Moon, labelKey: 'userMenu.themeDark' },
    { value: 'system', icon: Monitor, labelKey: 'userMenu.themeSystem' },
  ];

  const avatar = (
    <div className="flex size-8 items-center justify-center rounded-full bg-brand-secondary text-xs font-semibold text-fg-brand-secondary">
      DC
    </div>
  );

  const triggerButton = (
    <AriaButton
      className={cx(
        'flex w-full items-center gap-3 rounded-lg px-2 py-2 outline-hidden transition-colors hover:bg-primary_hover cursor-pointer',
        collapsed && 'justify-center',
      )}
    >
      {avatar}
      {!collapsed && (
        <>
          <div className="min-w-0 flex-1 text-left">
            <p className="truncate text-sm font-semibold text-secondary">DevClaw</p>
          </div>
          <ChevronUp className="size-4 shrink-0 text-fg-quaternary" />
        </>
      )}
    </AriaButton>
  );

  return (
    <div className="border-t border-secondary p-3">
      <AriaMenuTrigger>
        {collapsed ? (
          <Tooltip title={t('sidebar.menu')} placement="right" delay={200}>
            {triggerButton}
          </Tooltip>
        ) : (
          triggerButton
        )}

        <Dropdown.Popover
          placement={collapsed ? 'right bottom' : 'top start'}
          className="w-72"
        >
          <Dropdown.Menu>
            {/* Theme picker */}
            <Dropdown.Section>
              <Dropdown.SectionHeader className="px-4 pt-2 pb-1.5 text-xs font-semibold uppercase tracking-wider text-quaternary">
                {t('userMenu.theme', 'Theme')}
              </Dropdown.SectionHeader>
              <Dropdown.Item id="theme-picker" unstyled>
                <div className="px-3 pb-2">
                  <ThemePicker
                    theme={theme}
                    options={themeOptions}
                    onSelect={setTheme}
                    t={t}
                  />
                </div>
              </Dropdown.Item>
            </Dropdown.Section>

            <Dropdown.Separator />

            {/* Language picker */}
            <Dropdown.Section>
              <Dropdown.SectionHeader className="px-4 pt-2 pb-1.5 text-xs font-semibold uppercase tracking-wider text-quaternary">
                {t('userMenu.language', 'Language')}
              </Dropdown.SectionHeader>
              <Dropdown.Item id="lang-picker" unstyled>
                <div className="px-3 pb-2">
                  <LanguagePicker
                    currentCode={i18n.language}
                    onSelect={changeLanguage}
                  />
                </div>
              </Dropdown.Item>
            </Dropdown.Section>

            <Dropdown.Separator />

            {/* Actions */}
            <Dropdown.Item
              id="settings"
              label={t('sidebar.settings')}
              icon={Settings}
              onAction={handleSettings}
            />
            <Dropdown.Item
              id="logout"
              label={t('userMenu.logout')}
              icon={LogOut}
              onAction={handleLogout}
            />
          </Dropdown.Menu>
        </Dropdown.Popover>
      </AriaMenuTrigger>
    </div>
  );
}

/* ---- Theme Picker (pill group) ---- */

function ThemePicker({
  theme,
  options,
  onSelect,
  t,
}: {
  theme: Theme;
  options: { value: Theme; icon: typeof Sun; labelKey: string }[];
  onSelect: (v: Theme) => void;
  t: (k: string) => string;
}) {
  return (
    <div className="flex w-full gap-0.5 rounded-lg bg-quaternary p-0.5">
      {options.map(({ value, icon: Icon, labelKey }) => (
        <button
          key={value}
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onSelect(value);
          }}
          className={cx(
            'flex flex-1 items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium outline-hidden transition-all cursor-pointer',
            theme === value
              ? 'bg-primary text-secondary shadow-xs'
              : 'text-quaternary hover:text-tertiary',
          )}
          aria-label={t(labelKey)}
        >
          <Icon className="size-3.5" />
          <span className="hidden sm:inline">{t(labelKey)}</span>
        </button>
      ))}
    </div>
  );
}

/* ---- Language Picker (pill group) ---- */

function LanguagePicker({
  currentCode,
  onSelect,
}: {
  currentCode: string;
  onSelect: (code: string) => void;
}) {
  return (
    <div className="flex w-full gap-0.5 rounded-lg bg-quaternary p-0.5">
      {languages.map((lang) => (
        <button
          key={lang.code}
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onSelect(lang.code);
          }}
          className={cx(
            'flex flex-1 items-center justify-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium outline-hidden transition-all cursor-pointer',
            lang.code === currentCode
              ? 'bg-primary text-secondary shadow-xs'
              : 'text-quaternary hover:text-tertiary',
          )}
        >
          <span>{lang.code.toUpperCase()}</span>
        </button>
      ))}
    </div>
  );
}
